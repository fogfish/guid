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

package guid_test

import (
	"os"
	"testing"
	"time"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

func TestWithNodeID(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xfedcba98),
	)
	a := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Node(), 0xfedcba98),
	)
}

func TestWithNodeFromEnv(t *testing.T) {
	os.Setenv("CONFIG_GUID_NODE_ID", "abc@go")

	c := guid.NewClock(
		guid.WithNodeFromEnv(),
	)
	a := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Node(), 0x53051caf),
	)
}

func TestWithNodeRand(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeRandom(),
	)
	a := guid.NewG(c)

	it.Then(t).ShouldNot(
		it.Equal(a.Node(), 0x0),
	)
}

func TestWithClock(t *testing.T) {
	c := guid.NewClock(
		guid.WithClock(func() uint64 { return 0xfedcba98 << 16 }),
	)
	a := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Time(), 0xfedcba98<<16),
	)
}

func TestWithClockUnix(t *testing.T) {
	c := guid.NewClock(
		guid.WithClockUnix(),
	)
	a := guid.NewG(c)
	b := guid.NewG(c)
	time.Sleep(2 * time.Second)
	d := guid.NewG(c)

	it.Then(t).Should(
		it.True(a.Before(b)),
		it.True(b.Before(d)),
	)
}

func TestWithClockInverse(t *testing.T) {
	c := guid.NewClock(
		guid.WithClockInverse(),
	)
	a := guid.NewG(c)
	b := guid.NewG(c)
	time.Sleep(2 * time.Second)
	d := guid.NewG(c)

	it.Then(t).Should(
		it.True(a.After(b)),
		it.True(b.After(d)),
	)
}

func TestWithMock(t *testing.T) {
	c := guid.NewClockMock(
		guid.WithNodeID(0x0),
	)
	a := guid.NewG(c)
	b := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Node(), 0),
		it.Equal(a.Time(), 0),
		it.Equal(a.Seq(), 0),
		// the mock is deterministic, it repeats the very same value
		it.Equal(a, b),
		it.Equal(guid.NewL(c), guid.NewL(c)),
	)
}
