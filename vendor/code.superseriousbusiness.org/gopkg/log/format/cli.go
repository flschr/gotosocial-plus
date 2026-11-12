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

package format

import (
	"time"

	"code.superseriousbusiness.org/gopkg/log/level"
	"codeberg.org/gruf/go-byteutil"
	"codeberg.org/gruf/go-kv/v2"
	"codeberg.org/gruf/go-kv/v2/format"
)

// CLI wraps an existing FormatFunc to strip away much
// of the extra log formatting fields when level = UNSET.
type CLI struct{ FormatFunc }

func (cli CLI) Format(buf *byteutil.Buffer, stamp time.Time, pc uintptr, lvl level.LEVEL, kvs []kv.Field, msg string) {
	if lvl == level.UNSET {
		// Append formatted fields.
		for _, field := range kvs {
			kv.AppendQuoteString(buf, field.K)
			buf.B = append(buf.B, '=')
			buf.B = format.Global.Append(buf.B, field.V, format.Args{})
			buf.B = append(buf.B, ' ')
		}

		// Only output log message.
		buf.B = append(buf.B, msg...)
	} else {
		// For logging at level, passing to wrapped.
		cli.FormatFunc(buf, stamp, pc, lvl, kvs, msg)
	}
}
