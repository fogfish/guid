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

package guid

import (
	"crypto/rand"
	"errors"
	"testing"
)

// defaultNode's env-set branch is otherwise only reachable through
// Clock/Unclock's package-level initialization, which runs once before any
// test can set CONFIG_GUID_NODE_ID. clock_test.go's TestDefaultNodeFromEnv
// works around that by re-executing the test binary in a subprocess with the
// variable set, but a spawned subprocess's execution never merges into this
// process's coverage profile, so the branch shows as uncovered regardless.
// Calling defaultNode directly, from inside the package, needs neither the
// subprocess nor the timing workaround.
func TestDefaultNodeReadsEnvWhenSet(t *testing.T) {
	t.Setenv("CONFIG_GUID_NODE_ID", "abc@go")

	if got, want := defaultNode(), nodeFromEnvValue("abc@go"); got != want {
		t.Fatalf("got %#x, want %#x", got, want)
	}
}

// The empty-string case is folded into the same branch as unset: defaultNode
// treats both as "not configured" and falls back to random.
func TestDefaultNodeFallsBackToRandomWhenUnset(t *testing.T) {
	t.Setenv("CONFIG_GUID_NODE_ID", "")

	a, b := defaultNode(), defaultNode()
	if a == b {
		t.Fatalf("expected two random draws to differ, got %#x twice", a)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("crypto/rand unavailable") }

// randomNode's panic is defensive, not a case that happens in practice --
// crypto/rand.Reader failing on a supported platform essentially never does
// -- only one the code refuses to silently allocate a predictable node
// under. Swapping the package-level reader is the standard way to exercise
// that branch; it is restored before this test returns, and the test must
// not run in parallel with anything that also allocates a random node while
// the swap is in effect (nothing in this suite does).
func TestRandomNodePanicsWhenRandUnavailable(t *testing.T) {
	old := rand.Reader
	rand.Reader = failingReader{}
	defer func() { rand.Reader = old }()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected randomNode to panic when crypto/rand fails")
		}
	}()

	randomNode()
}
