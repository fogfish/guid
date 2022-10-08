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

	"github.com/fogfish/guid"
	"github.com/fogfish/it"
)

func TestConfNodeID(t *testing.T) {
	c := guid.NewLClock(
		guid.ConfNodeID(0xfedcba98),
	)
	a := guid.G.K(c)

	it.Ok(t).
		If(guid.G.Node(a)).Should().Equal(uint64(0xfedcba98))
}

func TestConfNodeFromEnv(t *testing.T) {
	os.Setenv("CONFIG_GUID_NODE_ID", "abc@go")

	c := guid.NewLClock(
		guid.ConfNodeFromEnv(),
	)
	a := guid.G.K(c)

	it.Ok(t).
		If(guid.G.Node(a)).Should().Equal(uint64(0x53051caf))
}

func TestConfNodeRand(t *testing.T) {
	c := guid.NewLClock(
		guid.ConfNodeRand(),
	)
	a := guid.G.K(c)

	it.Ok(t).
		If(guid.G.Node(a)).ShouldNot().Equal(0)
}

func TestConfClock(t *testing.T) {
	c := guid.NewLClock(
		guid.ConfClock(func() uint64 { return 0xfedcba98 << 16 }),
	)
	a := guid.G.K(c)

	it.Ok(t).
		If(guid.G.Time(a)).Should().Equal(uint64(0xfedcba98 << 16))
}

func TestConfClockUnix(t *testing.T) {
	c := guid.NewLClock(
		guid.ConfClockUnix(),
	)
	a := guid.G.K(c)
	b := guid.G.K(c)
	time.Sleep(2 * time.Second)
	d := guid.G.K(c)

	it.Ok(t).
		If(guid.G.Less(a, b)).Should().Equal(true).
		If(guid.G.Less(b, d)).Should().Equal(true)
}

func TestConfClockInverse(t *testing.T) {
	c := guid.NewLClock(
		guid.ConfClockInverse(),
	)
	a := guid.G.K(c)
	b := guid.G.K(c)
	time.Sleep(2 * time.Second)
	d := guid.G.K(c)

	it.Ok(t).
		If(guid.G.Less(b, a)).Should().Equal(true).
		If(guid.G.Less(d, b)).Should().Equal(true)
}
