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
	"time"

	"github.com/fogfish/guid/v3"
)

// Decoders and casts build their destination in place, so a call is a
// statement. Assertions want an expression, so the tests wrap them once here.
//
// Each wrapper is written out rather than folded into one generic, because the
// one thing they must not do is this:
//
//	return v, v.FromString(s)   // WRONG
//
// The order in which Go evaluates return operands relative to the calls among
// them is unspecified, so v may be copied before FromString has written to it.
// The temporary and the explicit err are load-bearing.

func fromStringG(val string) (guid.G, error) {
	var uid guid.G
	err := uid.FromString(val)
	return uid, err
}

func fromBytesG(val []byte) (guid.G, error) {
	var uid guid.G
	err := uid.FromBytes(val)
	return uid, err
}

func fromBase62G(val string) (guid.G, error) {
	var uid guid.G
	err := uid.FromBase62(val)
	return uid, err
}

func foldG(n uint64, val []byte) guid.G {
	var uid guid.G
	uid.Fold(n, val)
	return uid
}

func fromTimeG(clock guid.Chronos, t time.Time) guid.G {
	var uid guid.G
	uid.FromTime(clock, t)
	return uid
}

func gFromL(clock guid.Chronos, val guid.L) guid.G {
	var uid guid.G
	uid.FromL(clock, val)
	return uid
}

func gFromX(val guid.X) (guid.G, error) {
	var uid guid.G
	err := uid.FromX(val)
	return uid, err
}

func fromStringL(val string) (guid.L, error) {
	var uid guid.L
	err := uid.FromString(val)
	return uid, err
}

func fromBytesL(val []byte) (guid.L, error) {
	var uid guid.L
	err := uid.FromBytes(val)
	return uid, err
}

func fromBase62L(val string) (guid.L, error) {
	var uid guid.L
	err := uid.FromBase62(val)
	return uid, err
}

func foldL(n uint64, val []byte) guid.L {
	var uid guid.L
	uid.Fold(n, val)
	return uid
}

func fromTimeL(clock guid.Chronos, t time.Time) guid.L {
	var uid guid.L
	uid.FromTime(clock, t)
	return uid
}

func lFromG(val guid.G) guid.L {
	var uid guid.L
	uid.FromG(val)
	return uid
}

func lFromX(val guid.X) guid.L {
	var uid guid.L
	uid.FromX(val)
	return uid
}

func fromStringX(val string) (guid.X, error) {
	var uid guid.X
	err := uid.FromString(val)
	return uid, err
}

func fromBytesX(val []byte) (guid.X, error) {
	var uid guid.X
	err := uid.FromBytes(val)
	return uid, err
}

func fromBase62X(val string) (guid.X, error) {
	var uid guid.X
	err := uid.FromBase62(val)
	return uid, err
}

func foldX(n uint64, val []byte) guid.X {
	var uid guid.X
	uid.Fold(n, val)
	return uid
}

func fromTimeX(clock guid.Chronos, t time.Time) guid.X {
	var uid guid.X
	uid.FromTime(clock, t)
	return uid
}

func xFromG(val guid.G) guid.X {
	var uid guid.X
	uid.FromG(val)
	return uid
}

func xFromL(clock guid.Chronos, val guid.L) guid.X {
	var uid guid.X
	uid.FromL(clock, val)
	return uid
}
