//
//   Copyright 2012 Dmitry Kolesnikov, All Rights Reserved
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.
//

package guid

var alphabet []rune = []rune{
	'.', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'A', 'B', 'C', 'D', 'E',
	'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U',
	'V', 'W', 'X', 'Y', 'Z', '_', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j',
	'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
}

func encode64(uid UID) string {
	b := make([]rune, 16)
	for i, x := range uid.Split(6) {
		b[i] = alphabet[x]
	}
	return string(b)
}

func decode64(uid string) (val UID) {
	b := make([]byte, 16)
	for i, x := range uid {
		switch {
		case x == '.':
			b[i] = 0
		case x >= '0' && x <= '9':
			b[i] = byte(x-'0') + 1
		case x >= 'A' && x <= 'Z':
			b[i] = byte(x-'A') + 11
		case x == '_':
			b[i] = 37
		case x >= 'a' && x <= 'z':
			b[i] = byte(x-'a') + 38
		}
	}

	val.Fold(6, b)
	return
}
