// Command save-nodes writes all 6 nomadnet node entries to the directory
// file, so every node has all 5 peers saved as Known Nodes. Run it on
// each machine after stopping gonomadnet, then restart gonomadnet.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
)

type node struct {
	hash string
	name string
}

var allNodes = []node{
	{"bc37348ec27fafad10f3fd2e92ecf5f5", "Go port of NomadNet on Mac M2 Max"},
	{"cbae8e4890c9ca51d32b349d860fa977", "Go port of NomadNet on Linux Mint"},
	{"9554d5d4cb3be4b24a06dcf3be98e1d2", "Go port of NomadNet on Mac Mini M2"},
	{"9285a82e722bf4eeb7803f8f5fa202f0", "Go port of NomadNet on Jetson Nano2GB"},
	{"6853937554960093a05764c3974f28e6", "Go port of NomadNet on PixelBook"},
	{"5d292af53814640573c579ed51c7abd9", "Go port of NomadNet on RaspPi"},
}

func main() {
	home, _ := os.UserHomeDir()
	dirPath := filepath.Join(home, ".nomadnetwork", "storage", "directory")

	d := directory.New()
	if err := d.LoadFromDisk(dirPath); err != nil {
		fmt.Fprintf(os.Stderr, "load (ok if new): %v\n", err)
	}

	selfHash := ""
	if len(os.Args) > 1 {
		selfHash = os.Args[1]
	}

	saved := 0
	for _, n := range allNodes {
		if n.hash == selfHash {
			continue
		}
		hashBytes, err := hex.DecodeString(n.hash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad hash %v: %v\n", n.hash, err)
			continue
		}
		entry := directory.NewEntry(hashBytes)
		entry.DisplayName = n.name
		entry.HostsNode = true
		d.Remember(entry)
		saved++
		fmt.Printf("Saved: %v (%v)\n", n.name, n.hash)
	}

	d.SetPersistPath(dirPath)
	if err := d.SaveToDisk(dirPath); err != nil {
		fmt.Fprintf(os.Stderr, "save error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Done: saved %d nodes to %v\n", saved, dirPath)
}
