// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

//go:build wago && (linux || darwin || windows) && (amd64 || arm64)

// This file is the wago .wasm page renderer, compiled with -tags wago on the
// supported desktop platforms (Linux, Darwin, and Windows on amd64/arm64).
// Each render loads a sandboxed wasm plugin in-process (deny-by-default host
// imports, bounded linear memory and tables, hard per-invocation execution
// budget enforced by the runtime's interrupt mechanism), runs the
// render_page ABI, and releases the plugin before returning. The wired host
// imports are rns.log and the page-scoped rns.kv_get/rns.kv_set store; an
// import the host does not provide refuses instantiation.

package wasmpages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gmlewis/go-reticulum/pluginstore"
	wago "github.com/wago-org/wago/src/wago"
)

// pluginMaxMemoryBytes caps a page module's linear memory at admission.
const pluginMaxMemoryBytes = 16 * 1024 * 1024

// pluginMaxTableEntries caps a page module's table entries at admission.
const pluginMaxTableEntries = 1024

// DefaultTimeout is the per-invocation execution budget for a page plugin.
const DefaultTimeout = 2 * time.Second

// wasmPageRuntime owns one loaded page plugin (runtime, module, instance) so
// every render gets a fresh, stateless instance that is released afterwards.
type wasmPageRuntime struct {
	rt  *wago.Runtime
	mod *wago.Module
	// inst is non-nil after a successful start.
	inst *wago.Instance
}

// loadPlugin compiles and instantiates the wasm module bytes with the given
// host import surface.
func loadPlugin(wasmBytes []byte, imports wago.Imports) (*wasmPageRuntime, error) {
	w := &wasmPageRuntime{rt: wago.NewRuntime()}
	mod, err := w.rt.Compile(wasmBytes)
	if err != nil {
		_ = w.rt.Close()
		return nil, fmt.Errorf("wasm page compile: %w", err)
	}
	w.mod = mod
	if err := w.start(imports); err != nil {
		_ = w.mod.Close()
		_ = w.rt.Close()
		return nil, err
	}
	return w, nil
}

// start instantiates the compiled module with the admission policy and the
// host import surface. Anything the module imports but the host does not wire
// is refused here, so the deny-by-default policy holds even as capabilities
// are added.
func (w *wasmPageRuntime) start(imports wago.Imports) error {
	policy := wago.Policy{
		MaxMemoryBytes:  pluginMaxMemoryBytes,
		MaxTableEntries: pluginMaxTableEntries,
	}
	instantiateCtx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	inst, err := w.rt.Instantiate(instantiateCtx, w.mod, wago.WithPolicy(policy), wago.WithImports(imports))
	if err != nil {
		return fmt.Errorf("wasm page instantiate: %w", err)
	}
	w.inst = inst
	return nil
}

// pageImports builds the host import surface for the page plugin at filePath:
// rns.log forwards a (ptr, len) message to the installed logger, and
// rns.kv_set / rns.kv_get reach a KV scratch store scoped to this page's own
// directory, <pages-path>/data/<page>/ (the page file's base name). Anything
// else the module imports stays unwired and refuses instantiation.
func pageImports(filePath string) wago.Imports {
	imports := wago.Imports{
		"rns.log": wago.HostFunc(func(m wago.HostModule, params, _ []uint64) {
			logf := guestLogger()
			if logf == nil {
				return
			}
			mem := m.Memory()
			ptr, length := uint32(params[0]), uint32(params[1])
			if length == 0 || int(ptr)+int(length) > len(mem) {
				return
			}
			logf("wasm page %v: %s", filepath.Base(filePath), string(mem[ptr:ptr+length]))
		}),
	}
	addStoreImports(imports, pageStore(filePath))
	return imports
}

// pageStore opens the KV scratch store for the page plugin at filePath,
// scoped under <pages-path>/data/<page>/ by the page file's base name. A page
// whose base name is not a valid store name (a space or a slash in the file
// name) gets no store, so its KV imports report failure rather than sharing
// another page's data.
func pageStore(filePath string) *pluginstore.Store {
	base := filepath.Base(filePath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	store, err := pluginstore.New(filepath.Join(filepath.Dir(filePath), "data"), name)
	if err != nil {
		if logf := guestLogger(); logf != nil {
			logf("wasm page %v: KV store unavailable: %v", base, err)
		}
		return nil
	}
	return store
}

// addStoreImports wires a plugin's KV scratch store into the import surface:
// rns.kv_set stores a value (status 0 = ok, 1 = error) and rns.kv_get reads
// one (n = bytes written, 0 = missing key, -1 = output buffer too small;
// nothing is written partially). Keys and pointers are bounds-checked against
// guest memory.
func addStoreImports(imports wago.Imports, store *pluginstore.Store) {
	imports["rns.kv_set"] = wago.HostFunc(func(m wago.HostModule, params, results []uint64) {
		results[0] = 1
		if store == nil {
			return
		}
		mem := m.Memory()
		kPtr, kLen, vPtr, vLen := uint32(params[0]), uint32(params[1]), uint32(params[2]), uint32(params[3])
		if int(kPtr)+int(kLen) > len(mem) || int(vPtr)+int(vLen) > len(mem) {
			return
		}
		if err := store.Set(string(mem[kPtr:kPtr+kLen]), mem[vPtr:vPtr+vLen]); err != nil {
			return
		}
		results[0] = 0
	})
	imports["rns.kv_get"] = wago.HostFunc(func(m wago.HostModule, params, results []uint64) {
		results[0] = 0
		if store == nil {
			return
		}
		mem := m.Memory()
		kPtr, kLen, outPtr, outCap := uint32(params[0]), uint32(params[1]), uint32(params[2]), uint32(params[3])
		if int(kPtr)+int(kLen) > len(mem) {
			results[0] = 0xFFFFFFFF // -1 as i32
			return
		}
		value, ok, err := store.Get(string(mem[kPtr : kPtr+kLen]))
		if err != nil || !ok {
			return
		}
		if int(outCap) < len(value) || int(outPtr)+int(outCap) > len(mem) {
			results[0] = 0xFFFFFFFF // -1 as i32: caller retries with a bigger buffer
			return
		}
		copy(mem[outPtr:outPtr+uint32(len(value))], value)
		results[0] = uint64(uint32(len(value)))
	})
}

// close releases the instance, module, and runtime.
func (w *wasmPageRuntime) close() {
	if w.inst != nil {
		_ = w.inst.Close()
		w.inst = nil
	}
	if w.mod != nil {
		_ = w.mod.Close()
		w.mod = nil
	}
	if w.rt != nil {
		_ = w.rt.Close()
		w.rt = nil
	}
}

// call invokes a named export under the given execution budget.
func (w *wasmPageRuntime) call(export string, timeout time.Duration, args ...wago.Value) ([]wago.Value, error) {
	if w.inst == nil {
		return nil, errors.New("wasm page has no loaded plugin")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return w.inst.Call(ctx, export, args...)
}

// Enabled reports whether .wasm pages render through the sandbox.
func Enabled() bool { return true }

// Render runs the .wasm page plugin at filePath and returns its Micron
// markup: the request payload is allocated in guest memory through
// wagoplugin_alloc, the render_page (ptr, len) response is read back, and
// the markup bytes are returned.
func Render(filePath string, req PageRequest) ([]byte, error) {
	wasmBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("wasm page read %v: %w", filePath, err)
	}
	w, err := loadPlugin(wasmBytes, pageImports(filePath))
	if err != nil {
		return nil, err
	}
	defer w.close()

	payload := req.payload()
	allocRes, err := w.call("wagoplugin_alloc", DefaultTimeout, wago.ValueI32(int32(len(payload))))
	if err != nil {
		return nil, fmt.Errorf("wasm page alloc: %w", err)
	}
	if len(allocRes) < 1 {
		return nil, errors.New("wasm page plugin wagoplugin_alloc must return a pointer")
	}
	inPtr := uint32(allocRes[0].I32())
	if !w.inst.Write(inPtr, payload) {
		return nil, fmt.Errorf("wasm page memory write at %v (%v bytes) failed", inPtr, len(payload))
	}

	res, err := w.call("render_page", DefaultTimeout, allocRes[0], wago.ValueI32(int32(len(payload))))
	if err != nil {
		return nil, fmt.Errorf("wasm page render_page: %w", err)
	}
	if len(res) < 2 {
		return nil, errors.New("wasm page plugin render_page must return (ptr, len)")
	}
	outPtr, outLen := uint32(res[0].I32()), uint32(res[1].I32())
	markup, ok := w.inst.Read(outPtr, outLen)
	if !ok {
		return nil, fmt.Errorf("wasm page memory read at %v (%v bytes) failed", outPtr, outLen)
	}
	return markup, nil
}

// invokeExport loads the plugin at filePath, calls a named export with the
// given execution budget, and releases the plugin. It is the direct path the
// timeout tests exercise.
func invokeExport(filePath, export string, timeout time.Duration, args ...wago.Value) ([]wago.Value, error) {
	wasmBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("wasm page read %v: %w", filePath, err)
	}
	w, err := loadPlugin(wasmBytes, pageImports(filePath))
	if err != nil {
		return nil, err
	}
	defer w.close()
	return w.call(export, timeout, args...)
}
