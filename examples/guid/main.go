/*

  Copyright 2012 Dmitry Kolesnikov, All Rights Reserved

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

// Command guid allocates k-ordered identifiers and writes them to stdout, one
// per line.
//
// It is a demonstration of the three types the library defines and of the one
// property that is hard to see from a single value: run two instances side by
// side with different node identities and the output of each is a contiguous,
// individually ordered run of the key space.
//
// Usage:
//
//	guid [-l|-g|-x] [-n id] [-t delay] [-c count]
//
//	-l          allocate guid.L, 64-bit, unique within this process
//	-g          allocate guid.G, 96-bit, globally unique (the default)
//	-x          allocate guid.X, 128-bit, an RFC 9562 UUIDv8
//	-n id       node identity ⟨𝒍⟩; random when not given
//	-t delay    sleep a random interval in (0, delay] between allocations
//	-c count    number of identifiers to allocate, 0 for no limit
//
// The identifiers go to stdout and nothing else does, so the output pipes:
//
//	guid -x -c 1000 | sort -c && echo "allocated in sort order"
//	guid -g -n 1 -t 5ms -c 20
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/fogfish/guid/v3"
)

var (
	asL   = flag.Bool("l", false, "allocate guid.L, 64-bit, unique within this process")
	asG   = flag.Bool("g", false, "allocate guid.G, 96-bit, globally unique (the default)")
	asX   = flag.Bool("x", false, "allocate guid.X, 128-bit, an RFC 9562 UUIDv8")
	node  = flag.Uint64("n", 0, "node identity ⟨𝒍⟩, random when not given")
	delay = flag.Duration("t", 0, "sleep a random interval in (0, t] between allocations")
	count = flag.Int("c", 10, "number of identifiers to allocate, 0 for no limit")
)

func main() {
	flag.Parse()

	kind, err := allocator()
	if err != nil {
		fmt.Fprintf(os.Stderr, "guid: %s\n", err)
		flag.Usage()
		os.Exit(1)
	}

	clock := guid.NewClock(location())

	// the configuration goes to stderr so that stdout carries identifiers and
	// nothing else, and the pipeline stays usable
	fmt.Fprintf(os.Stderr, "guid: %s, node %s, drift %s\n",
		kind.name, kind.node(clock), clock.Drift())

	for i := 0; *count <= 0 || i < *count; i++ {
		sleep(*delay)
		fmt.Println(kind.next(clock))
	}
}

// kind is one of the three types the program can allocate.
type kind struct {
	name string
	// next allocates one identifier and renders it
	next func(guid.Chronos) string
	// node reports the ⟨𝒍⟩ fraction the type actually carries. It is not
	// always the clock's: the clock holds 58 bits, G truncates them to its own
	// 32, and L has no node field at all. FromTime asks the library that
	// question without drawing from the ⟨𝒕,𝒔⟩ sequencer.
	node func(guid.Chronos) string
}

// allocator picks the type to allocate from the -l, -g and -x flags. They name
// three different keyspaces rather than three renderings of one value, so at
// most one of them is meaningful per run.
func allocator() (kind, error) {
	l := kind{
		name: "guid.L",
		next: func(c guid.Chronos) string { return guid.NewL(c).String() },
		node: func(guid.Chronos) string { return "none (local value)" },
	}
	g := kind{
		name: "guid.G",
		next: func(c guid.Chronos) string { return guid.NewG(c).String() },
		node: func(c guid.Chronos) string {
			var uid guid.G
			uid.FromTime(c, time.Time{})
			return fmt.Sprintf("%d", uid.Node())
		},
	}
	x := kind{
		name: "guid.X",
		next: func(c guid.Chronos) string { return guid.NewX(c).String() },
		node: func(c guid.Chronos) string {
			var uid guid.X
			uid.FromTime(c, time.Time{})
			return fmt.Sprintf("%d", uid.Node())
		},
	}

	switch {
	case *asL && !*asG && !*asX:
		return l, nil
	case *asX && !*asL && !*asG:
		return x, nil
	case *asG && !*asL && !*asX:
		return g, nil
	case !*asL && !*asG && !*asX:
		return g, nil
	default:
		return kind{}, fmt.Errorf("-l, -g and -x are alternatives, give at most one")
	}
}

// location configures ⟨𝒍⟩ from -n, or leaves it random when the flag is absent.
//
// The flag's zero value cannot stand for "not given" — 0 is a node identity
// like any other, and one an operator assigning them by hand would reach for
// first — so the question is which flags were actually seen on the command
// line.
func location() guid.Config {
	given := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "n" {
			given = true
		}
	})

	if given {
		return guid.WithNodeID(*node)
	}

	return guid.WithNodeRandom()
}

// sleep waits a random interval in (0, max], and not at all for a max of zero.
//
// The interval is open at the bottom, so consecutive identifiers are always
// separated by at least a nanosecond of real time — which is still far below
// one tick of ⟨𝒕⟩, so ⟨𝒔⟩ is what orders them until the delay is large enough
// to cross a 131 µs boundary.
func sleep(max time.Duration) {
	if max <= 0 {
		return
	}

	time.Sleep(time.Duration(rand.Int63n(int64(max)) + 1))
}
