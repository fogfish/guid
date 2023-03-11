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

	"github.com/fogfish/guid/v2"
	"github.com/fogfish/it/v2"
)

func TestWithNodeID(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xfedcba98),
	)
	a := guid.G(c)

	it.Then(t).Should(
		it.Equal(guid.Node(a), 0xfedcba98),
	)
}

func TestWithNodeFromEnv(t *testing.T) {
	os.Setenv("CONFIG_GUID_NODE_ID", "abc@go")

	c := guid.NewClock(
		guid.WithNodeFromEnv(),
	)
	a := guid.G(c)

	it.Then(t).Should(
		it.Equal(guid.Node(a), 0x53051caf),
	)
}

func TestWithNodeRand(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeRandom(),
	)
	a := guid.G(c)

	it.Then(t).ShouldNot(
		it.Equal(guid.Node(a), 0x0),
	)
}

func TestWithClock(t *testing.T) {
	c := guid.NewClock(
		guid.WithClock(func() uint64 { return 0xfedcba98 << 16 }),
	)
	a := guid.G(c)

	it.Then(t).Should(
		it.Equal(guid.Time(a), 0xfedcba98<<16),
	)
}

func TestWithClockUnix(t *testing.T) {
	c := guid.NewClock(
		guid.WithClockUnix(),
	)
	a := guid.G(c)
	b := guid.G(c)
	time.Sleep(2 * time.Second)
	d := guid.G(c)

	it.Then(t).Should(
		it.True(guid.Before(a, b)),
		it.True(guid.Before(b, d)),
	)
}

func TestWithClockInverse(t *testing.T) {
	c := guid.NewClock(
		guid.WithClockInverse(),
	)
	a := guid.G(c)
	b := guid.G(c)
	time.Sleep(2 * time.Second)
	d := guid.G(c)

	it.Then(t).Should(
		it.True(guid.After(a, b)),
		it.True(guid.After(b, d)),
	)
}

func TestWithMock(t *testing.T) {
	c := guid.NewClockMock()
	a := guid.G(c)

	it.Then(t).Should(
		it.Equal(guid.Node(a), 0),
		it.Equal(guid.Time(a), 0),
		it.Equal(guid.Seq(a), 0),
	)
}
