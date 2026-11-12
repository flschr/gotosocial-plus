// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package log

import (
	"codeberg.org/gruf/go-kv/v2/format"
)

// LogFormatted provides log formatting
// of wrapped interface via String() method.
type LogFormatted struct{ any }

// String: implements fmt.Stringer{}.
func (f LogFormatted) String() string {
	buf := bufpool.Get()
	buf.B = format.Global.Append(buf.B, f.any, argArgs)
	str := string(buf.B)
	bufpool.Put(buf)
	return str
}

// GoString: implements fmt.GoStringer{}.
func (f LogFormatted) GoString() string {
	verboseArgs := argArgs
	verboseArgs.SetWithType()
	verboseArgs.SetNoMethod()
	buf := bufpool.Get()
	buf.B = format.Global.Append(buf.B, f.any, verboseArgs)
	str := string(buf.B)
	bufpool.Put(buf)
	return str
}

// Formatted wraps value in LogFormatted{}
// for nicer formatting via String() method.
func Formatted(v any) LogFormatted {
	return LogFormatted{v}
}
