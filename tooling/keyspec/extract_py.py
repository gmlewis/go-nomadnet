#!/usr/bin/env python3
# Extract the key-handling SPEC from the Python nomadnet source-of-truth UI
# tree by static AST analysis: every keypress / mouse_event / unhandled_input
# handler becomes a record of the keys it handles and the actions those keys
# trigger, plus the Ctrl-keys the shortcut bars advertise. The output is the
# machine-readable contract the Go port's key handling must match; it is
# derived fresh from the current Python source on every run.
#
# Usage: extract_py.py <nomadnet-repo-root>   → JSON on stdout.
# Pure stdlib; never imports nomadnet (the TUI must not run to build the spec).

import ast
import json
import os
import re
import sys

# UI files scanned, relative to the repo root (the TUI lives in ui/ +
# ui/textui/; everything else is non-UI infrastructure).
SCAN_SUBDIRS = [
    os.path.join("nomadnet", "ui"),
    os.path.join("nomadnet", "ui", "textui"),
]

# The key parameter name is the 3rd arg of keypress(self, size, key) and the
# 2nd of unhandled_input(self, key); the button is the 4th arg of mouse_event
# (self, size, event, button, x, y, focus).
KEY_METHODS = {"keypress": 3, "unhandled_input": 2}
MOUSE_METHOD = "mouse_event"


def attr_chain(node):
    """Return the dotted attribute chain of an AST value, or None."""
    parts = []
    while isinstance(node, ast.Attribute):
        parts.append(node.attr)
        node = node.value
    if isinstance(node, ast.Name):
        parts.append(node.id)
        parts.reverse()
        return ".".join(parts)
    return None


def first_action(stmts):
    """Describe the first meaningful action in a handler branch: the first
    method call's name (self.delegate.foo(...) -> foo), an emit, an assignment
    to an attribute, or a return marker."""
    for node in stmts:
        if isinstance(node, ast.Expr) and isinstance(node.value, ast.Call):
            func = node.value.func
            if isinstance(func, ast.Attribute):
                if func.attr == "_emit":
                    args = node.value.args
                    if args and isinstance(args[0], ast.Constant):
                        return "emit:%s" % args[0].value
                    return "emit"
                if func.attr == "keypress":
                    return "super-or-child"
                name = attr_chain(func)
                return name.split(".")[-1] if name else func.attr
            if isinstance(func, ast.Name):
                return func.id
        if isinstance(node, ast.Assign):
            target = node.targets[0]
            name = attr_chain(target)
            if name:
                return "assign:" + name.split(".")[-1]
        if isinstance(node, ast.Return):
            return "return"
        if isinstance(node, (ast.If, ast.For, ast.While)):
            inner = first_action(getattr(node, "body", []))
            if inner:
                return inner
    return None


def branch_actions(if_node):
    """Yield (test, body) for an if/elif chain keyed on the key parameter."""
    node = if_node
    while isinstance(node, ast.If):
        yield node.test, node.body
        if isinstance(node.orelse, list) and len(node.orelse) == 1 and isinstance(node.orelse[0], ast.If):
            node = node.orelse[0]
        else:
            break


def keys_from_compare(node, key_name):
    """Extract handled key strings from an ast.Compare involving the key
    parameter: key == 'x', 'x' == key, key in ('a', 'b')."""
    keys = []
    left, op, right = node.left, node.ops, node.comparators

    def str_consts(value):
        if isinstance(value, ast.Constant) and isinstance(value.value, str):
            return [value.value]
        if isinstance(value, (ast.Tuple, ast.List)):
            return [e.value for e in value.elts if isinstance(e, ast.Constant) and isinstance(e.value, str)]
        return []

    # Names matching the parameter anywhere in the comparison.
    def mentions_key(n):
        for sub in ast.walk(n):
            if isinstance(sub, ast.Name) and sub.id == key_name:
                return True
        return False

    if isinstance(op[0], ast.Eq):
        if mentions_key(left):
            keys.extend(str_consts(right[0]))
        elif mentions_key(right[0]):
            keys.extend(str_consts(left))
    elif isinstance(op[0], ast.In):
        keys.extend(str_consts(right[0]) if mentions_key(left) else str_consts(left))
    return keys


def collect_keys(node, key_name, out):
    """Walk a branch body collecting key literals from comparisons (including
    or-ed comparisons like key == 'tab' or key == 'down')."""
    for sub in ast.walk(node):
        if isinstance(sub, ast.BoolOp):
            for value in sub.values:
                if isinstance(value, ast.Compare):
                    out.extend(keys_from_compare(value, key_name))
        elif isinstance(sub, ast.Compare):
            out.extend(keys_from_compare(sub, key_name))
        if isinstance(sub, ast.Call) and isinstance(sub.func, ast.Attribute):
            # self._command_map[key] != urwid.ACTIVATE → urwid ACTIVATE handling
            if sub.func.attr == "get" and isinstance(sub.func.value, ast.Attribute) and sub.func.value.attr == "_command_map":
                out.append("activate")


def extract_file(path, rel):
    tree = ast.parse(open(path, encoding="utf-8").read(), filename=path)
    handlers, advertised = [], []
    for cls in [n for n in ast.walk(tree) if isinstance(n, ast.ClassDef)]:
        for fn in [n for n in cls.body if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef))]:
            if fn.name in KEY_METHODS:
                args = [a.arg for a in fn.args.args]
                key_name = args[KEY_METHODS[fn.name] - 1] if len(args) >= KEY_METHODS[fn.name] else "key"
                records = []
                for stmt in fn.body:
                    if isinstance(stmt, ast.If):
                        for test, body in branch_actions(stmt):
                            keys = []
                            collect_keys(test, key_name, keys)
                            if keys:
                                action = first_action(body) or ""
                                for k in keys:
                                    records.append({"key": k, "action": action})
                if records:
                    handlers.append({
                        "file": rel, "class": cls.name, "method": fn.name,
                        "line": fn.lineno, "keys": sorted(records, key=lambda r: (r["key"], r["action"])),
                    })
            elif fn.name == MOUSE_METHOD:
                buttons = []
                for sub in ast.walk(fn):
                    if isinstance(sub, ast.Compare):
                        for comp in sub.comparators + [sub.left]:
                            if isinstance(comp, ast.Constant) and isinstance(comp.value, int):
                                buttons.append(comp.value)
                if buttons:
                    handlers.append({
                        "file": rel, "class": cls.name, "method": MOUSE_METHOD,
                        "line": fn.lineno,
                        "keys": sorted({"key": "mouse:%d" % b, "action": "click"} for b in buttons if b > 0),
                    })

    # Advertised shortcut bars: every string literal containing "[C-" in the
    # file, with the Ctrl-keys it advertises ("[C-x] Delete" -> ctrl x).
    for node in ast.walk(tree):
        if isinstance(node, ast.Constant) and isinstance(node.value, str) and "[C-" in node.value:
            toks = sorted(set(m.lower() for m in re.findall(r"\[C-([a-zA-Z])\]", node.value)))
            if toks:
                advertised.append({"file": rel, "line": node.lineno, "keys": toks, "bar": node.value.strip()})
    return handlers, advertised


def main():
    repo = sys.argv[1]
    all_handlers, all_advertised = [], []
    for sub in SCAN_SUBDIRS:
        d = os.path.join(repo, sub)
        if not os.path.isdir(d):
            continue
        for name in sorted(os.listdir(d)):
            if not name.endswith(".py"):
                continue
            path = os.path.join(d, name)
            handlers, advertised = extract_file(path, os.path.relpath(path, repo))
            all_handlers.extend(handlers)
            all_advertised.extend(advertised)
    all_handlers.sort(key=lambda h: (h["file"], h["class"], h["method"], h["line"]))
    all_advertised.sort(key=lambda a: (a["file"], a["line"]))
    json.dump({"handlers": all_handlers, "advertised": all_advertised}, sys.stdout, indent=1)


if __name__ == "__main__":
    import sys
    main()