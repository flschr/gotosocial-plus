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
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"code.superseriousbusiness.org/gopkg/buffers"
	"code.superseriousbusiness.org/gopkg/log/format"
	"code.superseriousbusiness.org/gopkg/log/level"
	"code.superseriousbusiness.org/gopkg/xslices"
	"codeberg.org/gruf/go-kv/v2"
	kvformat "codeberg.org/gruf/go-kv/v2/format"
)

var state = struct {
	level  level.LEVEL
	hooks  []func(context.Context, []kv.Field) []kv.Field
	format format.FormatFunc
	output func(lvl level.LEVEL, line []byte)
}{
	level:  level.UNSET,
	hooks:  nil,
	format: format.NewLogfmt(""),
	output: func(_ level.LEVEL, line []byte) {
		_, _ = os.Stdout.Write(line)
	},
}

// memory pool of log buffers.
var bufpool = buffers.Pool(1024)

// Level returns the
// currently set log.
func Level() LEVEL {
	return state.level
}

// SetLevel sets the max logging.
func SetLevel(lvl LEVEL) {
	state.level = lvl
}

// AddHook adds the given hook to the logger context hooks stack.
func AddHook(hook func(ctx context.Context, kvs []kv.Field) []kv.Field) {
	if hook == nil {
		return
	}
	state.hooks = append(state.hooks, hook)
}

// SetFormat sets the given format func for logger.
func SetFormat(fn format.FormatFunc) {
	if fn == nil {
		return
	}
	state.format = fn
}

// SetOutput sets the given output func for logger.
func SetOutput(fn func(lvl LEVEL, line []byte)) {
	if fn == nil {
		fn = func(LEVEL, []byte) {}
	}
	state.output = fn
}

// SetOutputCLI prepares the logger for more CLI command oriented
// logging. It replaces output func to output all levelled logging
// to stderr, and UNSET (i.e. Printf etc) to stdout. It also updates
// the formatting func to drop timestamp and caller on UNSET entries.
func SetOutputCLI() {
	state.format = (format.CLI{FormatFunc: state.format}).Format
	state.output = func(lvl level.LEVEL, line []byte) {
		if lvl == UNSET {
			_, _ = os.Stdout.Write(line)
		} else {
			_, _ = os.Stderr.Write(line)
		}
	}
}

// ExtractFields extracts logging fields from given
// context, nil context is supported and a no-op.
func ExtractFields(ctx context.Context) []kv.Field {
	if ctx != nil && len(state.hooks) > 0 {

		// Allocate fields slice for our context hooks.
		fields := make([]kv.Field, 0, len(state.hooks))

		// Pass context through our hooks.
		for _, hook := range state.hooks {
			fields = hook(ctx, fields)
		}

		return fields
	}

	return nil
}

// New starts a new log entry.
func New() Entry {
	return Entry{}
}

// WithContext returns a new prepared Entry{} with context.
func WithContext(ctx context.Context) Entry {
	return Entry{ctx: ctx}
}

// WithField returns a new prepared Entry{} with key-value field.
func WithField(key string, value any) Entry {
	return Entry{kvs: []kv.Field{{K: key, V: value}}}
}

// WithFields returns a new prepared Entry{} with key-value fields.
func WithFields(fields ...kv.Field) Entry {
	return Entry{kvs: fields}
}

// Trace will log formatted args as 'msg' field to the log at TRACE level.
func Trace(ctx context.Context, a ...any) {
	if TRACE < state.level {
		return
	}
	logf(ctx, TRACE, nil, "", a...)
}

// Tracef will log format string as 'msg' field to the log at TRACE level.
func Tracef(ctx context.Context, s string, a ...any) {
	if TRACE < state.level {
		return
	}
	logf(ctx, TRACE, nil, s, a...)
}

// TraceKV will log the one key-value field to the log at TRACE level.
func TraceKV(ctx context.Context, key string, value any) {
	if TRACE < state.level {
		return
	}
	logf(ctx, TRACE, []kv.Field{{K: key, V: value}}, "")
}

// TraceKVs will log key-value fields to the log at TRACE level.
func TraceKVs(ctx context.Context, kvs ...kv.Field) {
	if TRACE < state.level {
		return
	}
	logf(ctx, TRACE, kvs, "")
}

// Debug will log formatted args as 'msg' field to the log at DEBUG level.
func Debug(ctx context.Context, a ...any) {
	if DEBUG < state.level {
		return
	}
	logf(ctx, DEBUG, nil, "", a...)
}

// Debugf will log format string as 'msg' field to the log at DEBUG level.
func Debugf(ctx context.Context, s string, a ...any) {
	if DEBUG < state.level {
		return
	}
	logf(ctx, DEBUG, nil, s, a...)
}

// DebugKV will log the one key-value field to the log at DEBUG level.
func DebugKV(ctx context.Context, key string, value any) {
	if DEBUG < state.level {
		return
	}
	logf(ctx, DEBUG, []kv.Field{{K: key, V: value}}, "")
}

// DebugKVs will log key-value fields to the log at DEBUG level.
func DebugKVs(ctx context.Context, kvs ...kv.Field) {
	if DEBUG < state.level {
		return
	}
	logf(ctx, DEBUG, kvs, "")
}

// Info will log formatted args as 'msg' field to the log at INFO level.
func Info(ctx context.Context, a ...any) {
	if INFO < state.level {
		return
	}
	logf(ctx, INFO, nil, "", a...)
}

// Infof will log format string as 'msg' field to the log at INFO level.
func Infof(ctx context.Context, s string, a ...any) {
	if INFO < state.level {
		return
	}
	logf(ctx, INFO, nil, s, a...)
}

// InfoKV will log the one key-value field to the log at INFO level.
func InfoKV(ctx context.Context, key string, value any) {
	if INFO < state.level {
		return
	}
	logf(ctx, INFO, []kv.Field{{K: key, V: value}}, "")
}

// InfoKVs will log key-value fields to the log at INFO level.
func InfoKVs(ctx context.Context, kvs ...kv.Field) {
	if INFO < state.level {
		return
	}
	logf(ctx, INFO, kvs, "")
}

// Warn will log formatted args as 'msg' field to the log at WARN level.
func Warn(ctx context.Context, a ...any) {
	if WARN < state.level {
		return
	}
	logf(ctx, WARN, nil, "", a...)
}

// Warnf will log format string as 'msg' field to the log at WARN level.
func Warnf(ctx context.Context, s string, a ...any) {
	if WARN < state.level {
		return
	}
	logf(ctx, WARN, nil, s, a...)
}

// WarnKV will log the one key-value field to the log at WARN level.
func WarnKV(ctx context.Context, key string, value any) {
	if WARN < state.level {
		return
	}
	logf(ctx, WARN, []kv.Field{{K: key, V: value}}, "")
}

// WarnKVs will log key-value fields to the log at WARN level.
func WarnKVs(ctx context.Context, kvs ...kv.Field) {
	if WARN < state.level {
		return
	}
	logf(ctx, WARN, kvs, "")
}

// Error will log formatted args as 'msg' field to the log at ERROR level.
func Error(ctx context.Context, a ...any) {
	if ERROR < state.level {
		return
	}
	logf(ctx, ERROR, nil, "", a...)
}

// Errorf will log format string as 'msg' field to the log at ERROR level.
func Errorf(ctx context.Context, s string, a ...any) {
	if ERROR < state.level {
		return
	}
	logf(ctx, ERROR, nil, s, a...)
}

// ErrorKV will log the one key-value field to the log at ERROR level.
func ErrorKV(ctx context.Context, key string, value any) {
	if ERROR < state.level {
		return
	}
	logf(ctx, ERROR, []kv.Field{{K: key, V: value}}, "")
}

// ErrorKVs will log key-value fields to the log at ERROR level.
func ErrorKVs(ctx context.Context, kvs ...kv.Field) {
	if ERROR < state.level {
		return
	}
	logf(ctx, ERROR, kvs, "")
}

// Panic will log formatted args as 'msg' field to the log at PANIC level.
// This will then call panic causing the application to crash.
func Panic(ctx context.Context, a ...any) {
	defer panic(fmt.Sprint(a...))
	if PANIC < state.level {
		return
	}
	logf(ctx, PANIC, nil, "", a...)
}

// Panicf will log format string as 'msg' field to the log at PANIC level.
// This will then call panic causing the application to crash.
func Panicf(ctx context.Context, s string, a ...any) {
	defer panic(fmt.Sprintf(s, a...))
	if PANIC < state.level {
		return
	}
	logf(ctx, PANIC, nil, s, a...)
}

// PanicKV will log the one key-value field to the log at PANIC level.
// This will then call panic causing the application to crash.
func PanicKV(ctx context.Context, key string, value any) {
	defer panic(kv.Field{K: key, V: value}.String())
	if PANIC < state.level {
		return
	}
	logf(ctx, PANIC, []kv.Field{{K: key, V: value}}, "")
}

// PanicKVs will log key-value fields to the log at PANIC level.
// This will then call panic causing the application to crash.
func PanicKVs(ctx context.Context, kvs ...kv.Field) {
	defer panic(kv.Fields(kvs).String())
	if PANIC < state.level {
		return
	}
	logf(ctx, PANIC, kvs, "")
}

// Log will log formatted args as 'msg' field to the log at given level.
func Log(ctx context.Context, lvl LEVEL, a ...any) { //nolint:revive
	if lvl < state.level {
		return
	}
	logf(ctx, lvl, nil, "", a...)
}

// Logf will log format string as 'msg' field to the log at given level.
func Logf(ctx context.Context, lvl LEVEL, s string, a ...any) { //nolint:revive
	if lvl < state.level {
		return
	}
	logf(ctx, lvl, nil, s, a...)
}

// LogKV will log the one key-value field to the log at given level.
func LogKV(ctx context.Context, lvl LEVEL, key string, value any) { //nolint:revive
	if lvl < state.level {
		return
	}
	logf(ctx, lvl, []kv.Field{{K: key, V: value}}, "")
}

// LogKVs will log key-value fields to the log at given level.
func LogKVs(ctx context.Context, lvl LEVEL, kvs ...kv.Field) { //nolint:revive
	if lvl < state.level {
		return
	}
	logf(ctx, lvl, kvs, "")
}

// Print will log formatted args to the log output.
func Print(a ...any) {
	logf(nil, UNSET, nil, "", a...)
}

// Printf will log format string to the log output.
func Printf(s string, a ...any) {
	logf(nil, UNSET, nil, s, a...)
}

// PrintKV will log the one key-value field to the log.
func PrintKV(key string, value any) {
	logf(nil, UNSET, []kv.Field{{K: key, V: value}}, "")
}

// PrintKVs will log key-value fields to the log.
func PrintKVs(kvs ...kv.Field) {
	logf(nil, UNSET, kvs, "")
}

// args used when msg = "".
var argArgs = kvformat.Args{
	Flags: kvformat.TextMask,
	Int:   kvformat.IntArgs{Base: 10},
	Uint:  kvformat.IntArgs{Base: 10},
	Float: kvformat.FloatArgs{Fmt: 'g', Prec: -1},
	Complex: kvformat.ComplexArgs{
		Real: kvformat.FloatArgs{Fmt: 'g', Prec: -1},
		Imag: kvformat.FloatArgs{Fmt: 'g', Prec: -1},
	},
}

// a note on design implementation here:
//
// logf contains the main "meat" of our logging package. everything
// goes through here. we also know that logging configuration won't
// be changed outside of initialization, so we can do away with any
// concurrency protection on e.g. writer, formatting, etc.
//
// logf should be complex enough that it doesn't get inlined, but
// to be sure we include a compiler tag to prevent it. we do this
// so that *callers* of logf can instead perform the simple level
// checking / other bits, and hopefully be inlined themselves. this
// that then means all log level checking can easily be inlined into
// their calling locations, and so save on function calls to logf()
// when not needed in heavily used DEBUG / TRACE logging, instead
// only performing boolean operations which themselves are inlined.
//
//go:noinline
func logf(ctx context.Context, lvl LEVEL, fields []kv.Field, msg string, args ...any) {

	// Get log stamp.
	now := time.Now()

	// Get caller information.
	pcs := make([]uintptr, 1)
	_ = runtime.Callers(3, pcs)

	// Acquire buffer.
	buf := bufpool.Get()
	defer bufpool.Put(buf)

	if ctx != nil && len(state.hooks) > 0 {

		// Ensure fields slice has space for context hooks.
		fields = xslices.GrowJust(fields, len(state.hooks))

		// Pass context through our hooks.
		for _, hook := range state.hooks {
			fields = hook(ctx, fields)
		}
	}

	if msg == "" {
		if len(args) > 0 {
			// Format each arg to buf.
			for _, arg := range args {
				buf.B = kvformat.Global.Append(buf.B, arg, argArgs)
				buf.B = append(buf.B, ' ')
			}

			// Drop last added space.
			buf.B = buf.B[:len(buf.B)-1]

			// Get buf as string.
			msg = string(buf.B)
			buf.B = buf.B[:0]
		}
	} else if msg != "" {
		// Format the message string.
		msg = fmt.Sprintf(msg, args...)
	}

	// Append formatted
	// entry to buffer.
	state.format(buf,
		now,
		pcs[0],
		lvl,
		fields,
		msg,
	)

	// Ensure a final new-line char.
	if buf.B[len(buf.B)-1] != '\n' {
		buf.B = append(buf.B, '\n')
	}

	// Write to output func.
	state.output(lvl, buf.B)
}
