/*
Copyright 2023 The Vitess Authors.

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

package evalengine

import (
	"bytes"

	"vitess.io/vitess/go/mysql/collations"
	"vitess.io/vitess/go/mysql/collations/charset"
	"vitess.io/vitess/go/mysql/collations/colldata"
	"vitess.io/vitess/go/sqltypes"
	vtrpcpb "vitess.io/vitess/go/vt/proto/vtrpc"
	"vitess.io/vitess/go/vt/sqlparser"
	"vitess.io/vitess/go/vt/vterrors"
)

type (
	builtinChangeCase struct {
		CallExpr
		upcase  bool
		collate collations.ID
	}

	builtinCharLength struct {
		CallExpr
	}

	builtinLength struct {
		CallExpr
	}

	builtinASCII struct {
		CallExpr
	}

	builtinOrd struct {
		CallExpr
		collate collations.ID
	}

	builtinBitLength struct {
		CallExpr
	}

	builtinCollation struct {
		CallExpr
	}

	builtinWeightString struct {
		CallExpr
		Cast   string
		Len    int
		HasLen bool
	}

	builtinLeftRight struct {
		CallExpr
		collate collations.ID
		left    bool
	}

	builtinPad struct {
		CallExpr
		collate collations.ID
		left    bool
	}

	builtinStrcmp struct {
		CallExpr
		collate collations.ID
	}

	builtinTrim struct {
		CallExpr
		collate collations.ID
		trim    sqlparser.TrimType
	}

	builtinSubstring struct {
		CallExpr
		collate collations.ID
	}
)

var _ IR = (*builtinChangeCase)(nil)
var _ IR = (*builtinCharLength)(nil)
var _ IR = (*builtinLength)(nil)
var _ IR = (*builtinASCII)(nil)
var _ IR = (*builtinOrd)(nil)
var _ IR = (*builtinBitLength)(nil)
var _ IR = (*builtinCollation)(nil)
var _ IR = (*builtinWeightString)(nil)
var _ IR = (*builtinLeftRight)(nil)
var _ IR = (*builtinPad)(nil)
var _ IR = (*builtinTrim)(nil)

func (call *builtinChangeCase) eval(env *ExpressionEnv) (eval, error) {
	arg, err := call.arg1(env)
	if err != nil {
		return nil, err
	}

	switch e := arg.(type) {
	case nil:
		return nil, nil

	case evalNumeric:
		return evalToVarchar(e, call.collate, false)

	case *evalBytes:
		coll := colldata.Lookup(e.col.Collation)
		csa, ok := coll.(colldata.CaseAwareCollation)
		if !ok {
			return nil, vterrors.Errorf(vtrpcpb.Code_UNIMPLEMENTED, "not implemented")
		}
		var newcase []byte
		if call.upcase {
			newcase = csa.ToUpper(nil, e.bytes)
		} else {
			newcase = csa.ToLower(nil, e.bytes)
		}
		return newEvalText(newcase, e.col), nil

	default:
		return e, nil
	}
}

func (call *builtinChangeCase) compile(c *compiler) (ctype, error) {
	str, err := call.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip := c.compileNullCheck1(str)

	switch {
	case str.isTextual():
	default:
		c.asm.Convert_xc(1, sqltypes.VarChar, c.collation, 0, false)
	}

	c.asm.Fn_LUCASE(call.upcase)
	c.asm.jumpDestination(skip)

	return str, nil
}

func (call *builtinCharLength) eval(env *ExpressionEnv) (eval, error) {
	arg, err := call.arg1(env)
	if err != nil {
		return nil, err
	}
	switch e := arg.(type) {
	case nil:
		return nil, nil
	case *evalBytes:
		if sqltypes.IsBinary(e.SQLType()) {
			return newEvalInt64(int64(len(e.bytes))), nil
		}
		coll := colldata.Lookup(e.col.Collation)
		count := charset.Length(coll.Charset(), e.bytes)
		return newEvalInt64(int64(count)), nil
	default:
		return newEvalInt64(int64(len(e.ToRawBytes()))), nil
	}
}

func (call *builtinCharLength) compile(c *compiler) (ctype, error) {
	return c.compileFn_length(call.Arguments[0], c.asm.Fn_CHAR_LENGTH)
}

func (call *builtinLength) eval(env *ExpressionEnv) (eval, error) {
	arg, err := call.arg1(env)
	if err != nil {
		return nil, err
	}
	if arg == nil {
		return nil, nil
	}
	return newEvalInt64(int64(len(arg.ToRawBytes()))), nil
}

func (call *builtinLength) compile(c *compiler) (ctype, error) {
	return c.compileFn_length(call.Arguments[0], c.asm.Fn_LENGTH)
}

func (call *builtinBitLength) eval(env *ExpressionEnv) (eval, error) {
	arg, err := call.arg1(env)
	if err != nil {
		return nil, err
	}
	if arg == nil {
		return nil, nil
	}
	return newEvalInt64(int64(len(arg.ToRawBytes())) * 8), nil
}

func (call *builtinBitLength) compile(c *compiler) (ctype, error) {
	return c.compileFn_length(call.Arguments[0], c.asm.Fn_BIT_LENGTH)
}

func (call *builtinASCII) eval(env *ExpressionEnv) (eval, error) {
	arg, err := call.arg1(env)
	if err != nil {
		return nil, err
	}
	if arg == nil {
		return nil, nil
	}

	b, ok := arg.(*evalBytes)
	if !ok {
		b = evalToBinary(arg)
	}
	if len(b.bytes) == 0 {
		return newEvalInt64(0), nil
	}
	return newEvalInt64(int64(b.bytes[0])), nil
}

func (call *builtinASCII) compile(c *compiler) (ctype, error) {
	str, err := call.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip := c.compileNullCheck1(str)

	switch {
	case str.isTextual():
	default:
		c.asm.Convert_xb(1, sqltypes.VarBinary, 0, false)
	}

	c.asm.Fn_ASCII()
	c.asm.jumpDestination(skip)

	return ctype{Type: sqltypes.Int64, Col: collationNumeric, Flag: nullableFlags(str.Flag)}, nil
}

func charOrd(b []byte, coll collations.ID) int64 {
	if len(b) == 0 {
		return 0
	}
	cs := colldata.Lookup(coll).Charset()
	_, l := cs.DecodeRune(b)
	var r int64
	for i := 0; i < l; i++ {
		r = (r << 8) | int64(b[i])
	}
	return r
}

func (call *builtinOrd) eval(env *ExpressionEnv) (eval, error) {
	arg, err := call.arg1(env)
	if err != nil {
		return nil, err
	}
	if arg == nil {
		return nil, nil
	}

	c, ok := arg.(*evalBytes)
	if !ok {
		c, err = evalToVarchar(arg, call.collate, false)
		if err != nil {
			return nil, err
		}
	}

	return newEvalInt64(charOrd(c.bytes, c.col.Collation)), nil
}

func (call *builtinOrd) compile(c *compiler) (ctype, error) {
	str, err := call.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip := c.compileNullCheck1(str)

	col := call.collate
	switch {
	case str.isTextual():
		col = str.Col.Collation
	default:
		c.asm.Convert_xc(1, sqltypes.VarChar, call.collate, 0, false)
	}

	c.asm.Fn_ORD(col)
	c.asm.jumpDestination(skip)

	return ctype{Type: sqltypes.Int64, Col: collationNumeric, Flag: nullableFlags(str.Flag)}, nil
}

// maxRepeatLength is the maximum number of times a string can be repeated.
// This is based on how MySQL behaves here. The maximum value in MySQL is
// actually based on `max_allowed_packet`. The value here is the maximum
// for `max_allowed_packet` with 1GB.
// See https://dev.mysql.com/doc/refman/8.0/en/server-system-variables.html#sysvar_max_allowed_packet
//
// Practically though, this is really whacky anyway.
// There's 3 possible states:
//   - `<= max_allowed_packet` and actual packet  generated is `<= max_allowed_packet`. It works
//   - `<= max_allowed_packet` but the actual packet generated is `> max_allowed_packet` so it fails with an
//     error: `ERROR 2020 (HY000): Got packet bigger than 'max_allowed_packet' bytes` and the client gets disconnected.
//   - `> max_allowed_packet`, no error and returns `NULL`.
const maxRepeatLength = 1073741824

type builtinRepeat struct {
	CallExpr
	collate collations.ID
}

func (call *builtinRepeat) eval(env *ExpressionEnv) (eval, error) {
	arg1, arg2, err := call.arg2(env)
	if err != nil {
		return nil, err
	}
	if arg1 == nil || arg2 == nil {
		return nil, nil
	}

	text, ok := arg1.(*evalBytes)
	if !ok {
		text, err = evalToVarchar(arg1, call.collate, true)
		if err != nil {
			return nil, err
		}
	}

	repeat := evalToInt64(arg2).i
	if repeat < 0 {
		repeat = 0
	}
	if !validMaxLength(int64(len(text.bytes)), repeat) {
		return nil, nil
	}

	return newEvalText(bytes.Repeat(text.bytes, int(repeat)), text.col), nil
}

func validMaxLength(len, repeat int64) bool {
	if repeat <= 0 {
		return true
	}
	if len*repeat/repeat != len {
		// we have an overflow, can't be a valid length.
		return false
	}
	return len*repeat <= maxRepeatLength
}

func (expr *builtinRepeat) compile(c *compiler) (ctype, error) {
	str, err := expr.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	repeat, err := expr.Arguments[1].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip := c.compileNullCheck2(str, repeat)

	switch {
	case str.isTextual():
	default:
		c.asm.Convert_xc(2, sqltypes.VarChar, c.collation, 0, false)
	}
	_ = c.compileToInt64(repeat, 1)

	c.asm.Fn_REPEAT()
	c.asm.jumpDestination(skip)
	return ctype{Type: sqltypes.VarChar, Col: str.Col, Flag: flagNullable}, nil
}

func (c *builtinCollation) eval(env *ExpressionEnv) (eval, error) {
	arg, err := c.arg1(env)
	if err != nil {
		return nil, err
	}

	col := evalCollation(arg)

	// the collation of a `COLLATION` expr is hardcoded to `utf8mb3_general_ci`,
	// not to the default collation of our connection. this is probably a bug in MySQL, but we match it
	return newEvalText([]byte(env.collationEnv.LookupName(col.Collation)), collationUtf8mb3), nil
}

func (expr *builtinCollation) compile(c *compiler) (ctype, error) {
	_, err := expr.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip := c.asm.jumpFrom()

	c.asm.Fn_COLLATION(c.env.CollationEnv(), collationUtf8mb3)
	c.asm.jumpDestination(skip)

	return ctype{Type: sqltypes.VarChar, Col: collationUtf8mb3}, nil
}

func (c *builtinWeightString) eval(env *ExpressionEnv) (eval, error) {
	var weights []byte

	input, err := c.arg1(env)
	if err != nil {
		return nil, err
	}

	typ := sqltypes.VarBinary

	if c.Cast == "binary" {
		switch input.SQLType() {
		case sqltypes.Blob, sqltypes.Text, sqltypes.TypeJSON:
			typ = sqltypes.Blob
		}

		weights, _, err = evalWeightString(weights, evalToBinary(input), c.Len, 0)
		if err != nil {
			return nil, err
		}
		return newEvalRaw(typ, weights, collationBinary), nil
	}

	switch val := input.(type) {
	case *evalInt64, *evalUint64, *evalTemporal:
		weights, _, err = evalWeightString(weights, val, 0, 0)
	case *evalJSON:
		// JSON doesn't actually use a sortable weight string for this function, but
		// returns the weight string directly for the string based representation. This
		// means that ordering etc. is not correct for JSON values, but that's how MySQL
		// works here for this function. We still have the internal weight string logic
		// that can order these correctly.
		out, err := evalToVarchar(val, collationJSON.Collation, false)
		if err != nil {
			return nil, err
		}
		weights, _, err = evalWeightString(weights, out, 0, 0)
		if err != nil {
			return nil, err
		}
		typ = sqltypes.Blob
	case *evalBytes:
		switch val.SQLType() {
		case sqltypes.Blob, sqltypes.Text:
			typ = sqltypes.Blob
		}
		if val.isBinary() {
			weights, _, err = evalWeightString(weights, val, 0, 0)
		} else {
			var strLen int
			if c.Cast == "char" {
				strLen = c.Len
			}
			weights, _, err = evalWeightString(weights, val, strLen, 0)
		}
	default:
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return newEvalRaw(typ, weights, collationBinary), nil
}

func (call *builtinWeightString) compile(c *compiler) (ctype, error) {
	str, err := call.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	var flag typeFlag
	if str.Flag&flagNullable != 0 {
		flag = flag | flagNullable
	}

	typ := sqltypes.VarBinary
	skip := c.compileNullCheck1(str)
	if call.Cast == "binary" {
		if !sqltypes.IsBinary(str.Type) {
			c.asm.Convert_xb(1, sqltypes.VarBinary, 0, false)
		}
		switch str.Type {
		case sqltypes.Blob, sqltypes.Text, sqltypes.TypeJSON:
			typ = sqltypes.Blob
		}

		c.asm.Fn_WEIGHT_STRING(typ, call.Len)
		c.asm.jumpDestination(skip)
		return ctype{Type: sqltypes.VarBinary, Flag: flagNullable | flagNull, Col: collationBinary}, nil
	}

	switch str.Type {
	case sqltypes.Int64, sqltypes.Uint64, sqltypes.Date, sqltypes.Datetime, sqltypes.Timestamp, sqltypes.Time, sqltypes.VarBinary, sqltypes.Binary, sqltypes.Blob:
		if str.Type == sqltypes.Blob {
			typ = sqltypes.Blob
		}
		c.asm.Fn_WEIGHT_STRING(typ, 0)
	case sqltypes.TypeJSON:
		typ = sqltypes.Blob
		c.asm.Convert_xce(1, sqltypes.VarChar, collationJSON.Collation)
		c.asm.Fn_WEIGHT_STRING(typ, 0)
	case sqltypes.VarChar, sqltypes.Char, sqltypes.Text:
		if str.Type == sqltypes.Text {
			typ = sqltypes.Blob
		}
		var strLen int
		if call.Cast == "char" {
			strLen = call.Len
		}
		c.asm.Fn_WEIGHT_STRING(typ, strLen)

	default:
		c.asm.SetNull(1)
		flag = flag | flagNull | flagNullable
	}

	c.asm.jumpDestination(skip)
	return ctype{Type: typ, Flag: flag, Col: collationBinary}, nil
}

func (call *builtinLeftRight) eval(env *ExpressionEnv) (eval, error) {
	str, l, err := call.arg2(env)
	if err != nil {
		return nil, err
	}
	if str == nil || l == nil {
		return nil, nil
	}

	text, ok := str.(*evalBytes)
	if !ok {
		text, err = evalToVarchar(str, call.collate, true)
		if err != nil {
			return nil, err
		}
	}

	length := evalToInt64(l).i
	if length <= 0 {
		return newEvalText(nil, text.col), nil
	}

	// LEFT / RIGHT operates on characters, not bytes
	cs := colldata.Lookup(text.col.Collation).Charset()
	strLen := charset.Length(cs, text.bytes)

	if strLen <= int(length) {
		return newEvalText(text.bytes, text.col), nil
	}

	var res []byte
	if call.left {
		res = charset.Slice(cs, text.bytes, 0, int(length))
	} else {
		res = charset.Slice(cs, text.bytes, strLen-int(length), strLen)
	}
	return newEvalText(res, text.col), nil
}

func (call *builtinLeftRight) compile(c *compiler) (ctype, error) {
	str, err := call.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	l, err := call.Arguments[1].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip := c.compileNullCheck2(str, l)

	col := typedCoercionCollation(sqltypes.VarChar, c.collation)
	switch {
	case str.isTextual():
		col = str.Col
	default:
		c.asm.Convert_xc(2, sqltypes.VarChar, col.Collation, 0, false)
	}
	_ = c.compileToInt64(l, 1)

	if call.left {
		c.asm.Fn_LEFT(col)
	} else {
		c.asm.Fn_RIGHT(col)
	}
	c.asm.jumpDestination(skip)
	return ctype{Type: sqltypes.VarChar, Col: col, Flag: flagNullable}, nil
}

func (call *builtinPad) eval(env *ExpressionEnv) (eval, error) {
	str, l, p, err := call.arg3(env)
	if err != nil {
		return nil, err
	}

	if str == nil || l == nil || p == nil {
		return nil, nil
	}

	text, ok := str.(*evalBytes)
	if !ok {
		text, err = evalToVarchar(str, call.collate, true)
		if err != nil {
			return nil, err
		}
	}

	cs := colldata.Lookup(text.col.Collation).Charset()
	pad, ok := p.(*evalBytes)
	if !ok || colldata.Lookup(pad.col.Collation).Charset() != cs {
		pad, err = evalToVarchar(p, text.col.Collation, true)
		if err != nil {
			return nil, err
		}
	}

	length := evalToInt64(l).i
	if length < 0 {
		return nil, nil
	}

	if !validMaxLength(int64(len(pad.bytes)), length) {
		return nil, nil
	}

	// LPAD / RPAD operates on characters, not bytes
	strLen := charset.Length(cs, text.bytes)

	if strLen >= int(length) {
		// If the existing string is longer than the requested padding,
		// MySQL truncates the string to the requested padding length.
		return newEvalText(charset.Slice(cs, text.bytes, 0, int(length)), text.col), nil
	}

	runeLen := charset.Length(cs, pad.bytes)
	if runeLen == 0 {
		return newEvalText(nil, text.col), nil
	}

	repeat := (int(length) - strLen) / runeLen
	remainder := (int(length) - strLen) % runeLen

	var res []byte
	if !call.left {
		res = text.bytes
	}

	res = append(res, bytes.Repeat(pad.bytes, repeat)...)
	if remainder > 0 {
		res = append(res, charset.Slice(cs, pad.bytes, 0, remainder)...)
	}

	if call.left {
		res = append(res, text.bytes...)
	}

	return newEvalText(res, text.col), nil
}

func (call *builtinPad) compile(c *compiler) (ctype, error) {
	str, err := call.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	l, err := call.Arguments[1].compile(c)
	if err != nil {
		return ctype{}, err
	}

	pad, err := call.Arguments[2].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip := c.compileNullCheck3(str, l, pad)

	col := typedCoercionCollation(sqltypes.VarChar, c.collation)
	switch {
	case str.isTextual():
		col = str.Col
	default:
		c.asm.Convert_xce(3, sqltypes.VarChar, col.Collation)
	}
	_ = c.compileToInt64(l, 2)

	switch {
	case pad.isTextual():
		fromCharset := colldata.Lookup(pad.Col.Collation).Charset()
		toCharset := colldata.Lookup(col.Collation).Charset()
		if fromCharset != toCharset && !toCharset.IsSuperset(fromCharset) {
			c.asm.Convert_xce(1, sqltypes.VarChar, col.Collation)
		}
	default:
		c.asm.Convert_xce(1, sqltypes.VarChar, col.Collation)
	}

	if call.left {
		c.asm.Fn_LPAD(col)
	} else {
		c.asm.Fn_RPAD(col)
	}
	c.asm.jumpDestination(skip)
	return ctype{Type: sqltypes.VarChar, Flag: flagNullable, Col: col}, nil
}

func strcmpCollate(left, right []byte, col collations.ID) int64 {
	cmp := colldata.Lookup(col).Collate(left, right, false)
	switch {
	case cmp == 0:
		return 0
	case cmp > 0:
		return 1
	default:
		return -1
	}
}

func (l *builtinStrcmp) eval(env *ExpressionEnv) (eval, error) {
	left, err := l.Arguments[0].eval(env)
	if left == nil || err != nil {
		return nil, err
	}

	right, err := l.Arguments[1].eval(env)
	if right == nil || err != nil {
		return nil, err
	}

	if _, ok := left.(evalNumeric); ok {
		return newEvalInt64(strcmpCollate(left.ToRawBytes(), right.ToRawBytes(), collationNumeric.Collation)), nil
	}
	if _, ok := right.(evalNumeric); ok {
		return newEvalInt64(strcmpCollate(left.ToRawBytes(), right.ToRawBytes(), collationNumeric.Collation)), nil
	}

	col1 := evalCollation(left)
	col2 := evalCollation(right)

	mcol, _, _, err := colldata.Merge(env.collationEnv, col1, col2, colldata.CoercionOptions{
		ConvertToSuperset:   true,
		ConvertWithCoercion: true,
	})
	if err != nil {
		return nil, err
	}

	left, err = evalToVarchar(left, mcol.Collation, true)
	if err != nil {
		return nil, err
	}

	right, err = evalToVarchar(right, mcol.Collation, true)
	if err != nil {
		return nil, err
	}

	return newEvalInt64(strcmpCollate(left.ToRawBytes(), right.ToRawBytes(), mcol.Collation)), nil
}

func (expr *builtinStrcmp) compile(c *compiler) (ctype, error) {
	lt, err := expr.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip1 := c.compileNullCheck1(lt)

	rt, err := expr.Arguments[1].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip2 := c.compileNullCheck1r(rt)
	var mcol collations.TypedCollation

	if sqltypes.IsNumber(lt.Type) || sqltypes.IsNumber(rt.Type) {
		mcol = collationNumeric
	} else {
		mcol, _, _, err = colldata.Merge(c.env.CollationEnv(), lt.Col, rt.Col, colldata.CoercionOptions{
			ConvertToSuperset:   true,
			ConvertWithCoercion: true,
		})
		if err != nil {
			return ctype{}, err
		}
	}

	if !lt.isTextual() || lt.Col.Collation != mcol.Collation {
		c.asm.Convert_xce(2, sqltypes.VarChar, mcol.Collation)
	}

	if !rt.isTextual() || rt.Col.Collation != mcol.Collation {
		c.asm.Convert_xce(1, sqltypes.VarChar, mcol.Collation)
	}

	c.asm.Strcmp(mcol)
	c.asm.jumpDestination(skip1, skip2)
	return ctype{Type: sqltypes.Int64, Col: collationNumeric, Flag: nullableFlags(lt.Flag | rt.Flag)}, nil
}

func (call *builtinTrim) eval(env *ExpressionEnv) (eval, error) {
	str, err := call.arg1(env)
	if err != nil {
		return nil, err
	}

	if str == nil {
		return nil, nil
	}

	text, ok := str.(*evalBytes)
	if !ok {
		text, err = evalToVarchar(str, call.collate, true)
		if err != nil {
			return nil, err
		}
	}

	if len(call.Arguments) == 1 {
		switch call.trim {
		case sqlparser.LeadingTrimType:
			return newEvalText(bytes.TrimLeft(text.bytes, " "), text.col), nil
		case sqlparser.TrailingTrimType:
			return newEvalText(bytes.TrimRight(text.bytes, " "), text.col), nil
		default:
			return newEvalText(bytes.Trim(text.bytes, " "), text.col), nil
		}
	}

	p, err := call.Arguments[1].eval(env)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}

	pat, ok := p.(*evalBytes)
	if !ok || colldata.Lookup(pat.col.Collation).Charset() != colldata.Lookup(text.col.Collation).Charset() {
		pat, err = evalToVarchar(p, text.col.Collation, true)
		if err != nil {
			return nil, err
		}
	}

	switch call.trim {
	case sqlparser.LeadingTrimType:
		return newEvalText(bytes.TrimPrefix(text.bytes, pat.bytes), text.col), nil
	case sqlparser.TrailingTrimType:
		return newEvalText(bytes.TrimSuffix(text.bytes, pat.bytes), text.col), nil
	default:
		return newEvalText(bytes.TrimPrefix(bytes.TrimSuffix(text.bytes, pat.bytes), pat.bytes), text.col), nil
	}
}

func (call *builtinTrim) compile(c *compiler) (ctype, error) {
	str, err := call.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip1 := c.compileNullCheck1(str)

	col := typedCoercionCollation(sqltypes.VarChar, c.collation)
	switch {
	case str.isTextual():
		col = str.Col
	default:
		c.asm.Convert_xc(1, sqltypes.VarChar, col.Collation, 0, false)
	}

	if len(call.Arguments) == 1 {
		switch call.trim {
		case sqlparser.LeadingTrimType:
			c.asm.Fn_LTRIM1(col)
		case sqlparser.TrailingTrimType:
			c.asm.Fn_RTRIM1(col)
		default:
			c.asm.Fn_TRIM1(col)
		}
		c.asm.jumpDestination(skip1)
		return ctype{Type: sqltypes.VarChar, Flag: nullableFlags(str.Flag), Col: col}, nil
	}

	pat, err := call.Arguments[1].compile(c)
	if err != nil {
		return ctype{}, err
	}

	skip2 := c.compileNullCheck1r(pat)

	switch {
	case pat.isTextual():
		fromCharset := colldata.Lookup(pat.Col.Collation).Charset()
		toCharset := colldata.Lookup(col.Collation).Charset()
		if fromCharset != toCharset && !toCharset.IsSuperset(fromCharset) {
			c.asm.Convert_xce(1, sqltypes.VarChar, col.Collation)
		}
	default:
		c.asm.Convert_xce(1, sqltypes.VarChar, col.Collation)
	}

	switch call.trim {
	case sqlparser.LeadingTrimType:
		c.asm.Fn_LTRIM2(col)
	case sqlparser.TrailingTrimType:
		c.asm.Fn_RTRIM2(col)
	default:
		c.asm.Fn_TRIM2(col)
	}

	c.asm.jumpDestination(skip1, skip2)
	return ctype{Type: sqltypes.VarChar, Flag: flagNullable, Col: col}, nil
}

func (call *builtinSubstring) eval(env *ExpressionEnv) (eval, error) {
	str, err := call.Arguments[0].eval(env)
	if err != nil || str == nil {
		return nil, err
	}

	tt := str.SQLType()
	text, ok := str.(*evalBytes)
	if !ok {
		text, err = evalToVarchar(str, call.collate, true)
		if err != nil {
			return nil, err
		}
		tt = sqltypes.VarChar
	}

	p, err := call.Arguments[1].eval(env)
	if err != nil || p == nil {
		return nil, err
	}

	var l eval
	if len(call.Arguments) > 2 {
		l, err = call.Arguments[2].eval(env)
		if err != nil || l == nil {
			return nil, err
		}
	}

	pos := evalToInt64(p).i
	if pos == 0 {
		return newEvalRaw(tt, nil, text.col), nil
	}
	cs := colldata.Lookup(text.col.Collation).Charset()
	end := int64(charset.Length(cs, text.bytes))

	if pos < 0 {
		pos += end + 1
	}
	if pos < 1 || pos > end {
		return newEvalRaw(tt, nil, text.col), nil
	}

	if len(call.Arguments) > 2 {
		ll := evalToInt64(l).i
		if ll < 1 {
			return newEvalRaw(tt, nil, text.col), nil
		}
		if ll > end-pos+1 {
			ll = end - pos + 1
		}
		end = pos + ll - 1
	}
	res := charset.Slice(cs, text.bytes, int(pos-1), int(end))
	return newEvalRaw(tt, res, text.col), nil
}

func (call *builtinSubstring) compile(c *compiler) (ctype, error) {
	str, err := call.Arguments[0].compile(c)
	if err != nil {
		return ctype{}, err
	}

	p, err := call.Arguments[1].compile(c)
	if err != nil {
		return ctype{}, err
	}

	tt := str.Type
	skip1 := c.compileNullCheck2(str, p)

	col := typedCoercionCollation(sqltypes.VarChar, c.collation)
	switch {
	case str.isTextual():
		col = str.Col
	default:
		tt = sqltypes.VarChar
		c.asm.Convert_xc(2, tt, col.Collation, 0, false)
	}
	_ = c.compileToInt64(p, 1)

	cs := colldata.Lookup(str.Col.Collation).Charset()
	var skip2 *jump
	if len(call.Arguments) > 2 {
		l, err := call.Arguments[2].compile(c)
		if err != nil {
			return ctype{}, err
		}
		skip2 = c.compileNullCheck2(str, l)
		_ = c.compileToInt64(l, 1)
		c.asm.Fn_SUBSTRING3(tt, cs, col)
	} else {
		c.asm.Fn_SUBSTRING2(tt, cs, col)
	}

	c.asm.jumpDestination(skip1, skip2)
	return ctype{Type: tt, Col: col, Flag: flagNullable}, nil
}

type builtinConcat struct {
	CallExpr
	collate collations.ID
}

func concatSQLType(arg sqltypes.Type, tt sqltypes.Type) sqltypes.Type {
	if arg == sqltypes.TypeJSON {
		return sqltypes.Blob
	}

	if sqltypes.IsBinary(tt) {
		return tt
	}

	if sqltypes.IsBinary(arg) {
		return sqltypes.VarBinary
	}

	return sqltypes.VarChar
}

func concatConvert(buf []byte, str *evalBytes, tc collations.TypedCollation) ([]byte, error) {
	if tc.Collation == collations.CollationBinaryID {
		return append(buf, str.bytes...), nil
	}
	fromCharset := colldata.Lookup(str.col.Collation).Charset()
	toCharset := colldata.Lookup(tc.Collation).Charset()
	if fromCharset != toCharset {
		return charset.Convert(buf, toCharset, str.bytes, fromCharset)
	}
	return append(buf, str.bytes...), nil
}

func (call *builtinConcat) eval(env *ExpressionEnv) (eval, error) {
	var ca collationAggregation
	tt := sqltypes.VarChar

	args := make([]eval, 0, len(call.Arguments))
	for _, arg := range call.Arguments {
		a, err := arg.eval(env)
		if a == nil || err != nil {
			return nil, err
		}
		args = append(args, a)
		tt = concatSQLType(a.SQLType(), tt)

		err = ca.add(evalCollation(a), env.collationEnv)
		if err != nil {
			return nil, err
		}
	}

	tc := ca.result()
	// If we only had numbers, we instead fall back to the default
	// collation instead of using the numeric collation.
	if tc.Coercibility == collations.CoerceNumeric {
		tc = typedCoercionCollation(tt, call.collate)
	}

	var buf []byte
	for _, arg := range args {
		switch a := arg.(type) {
		case *evalBytes:
			var err error
			buf, err = concatConvert(buf, a, tc)
			if err != nil {
				return nil, err
			}
		default:
			c, err := evalToVarchar(a, tc.Collation, true)
			if err != nil {
				return nil, err
			}
			buf = append(buf, c.bytes...)
		}
	}

	return newEvalRaw(tt, buf, tc), nil
}

func (call *builtinConcat) compile(c *compiler) (ctype, error) {
	var ca collationAggregation
	tt := sqltypes.VarChar
	var f typeFlag

	args := make([]ctype, 0, len(call.Arguments))
	skips := make([]*jump, 0, len(call.Arguments))
	for i, arg := range call.Arguments {
		a, err := arg.compile(c)
		if err != nil {
			return ctype{}, err
		}
		f |= a.Flag
		skips = append(skips, c.compileNullCheckArg(a, i))
		args = append(args, a)
		tt = concatSQLType(a.Type, tt)

		err = ca.add(a.Col, c.env.CollationEnv())
		if err != nil {
			return ctype{}, err
		}
	}

	tc := ca.result()
	// If we only had numbers, we instead fall back to the default
	// collation instead of using the numeric collation.
	if tc.Coercibility == collations.CoerceNumeric {
		tc = typedCoercionCollation(tt, call.collate)
	}

	for i, arg := range args {
		switch arg.Type {
		case sqltypes.VarBinary, sqltypes.Binary, sqltypes.Blob:
			if tc.Collation != collations.CollationBinaryID {
				c.asm.Convert_xce(len(args)-i, arg.Type, tc.Collation)
			}
		case sqltypes.VarChar, sqltypes.Char, sqltypes.Text:
			fromCharset := colldata.Lookup(arg.Col.Collation).Charset()
			toCharset := colldata.Lookup(tc.Collation).Charset()
			if fromCharset != toCharset && !toCharset.IsSuperset(fromCharset) {
				c.asm.Convert_xce(len(args)-i, arg.Type, tc.Collation)
			}
		default:
			c.asm.Convert_xce(len(args)-i, arg.Type, tc.Collation)
		}
	}

	c.asm.Fn_CONCAT(tt, tc, len(args))
	c.asm.jumpDestination(skips...)

	return ctype{Type: tt, Flag: f, Col: tc}, nil
}

type builtinConcatWs struct {
	CallExpr
	collate collations.ID
}

func (call *builtinConcatWs) eval(env *ExpressionEnv) (eval, error) {
	var ca collationAggregation
	tt := sqltypes.VarChar

	args := make([]eval, 0, len(call.Arguments))
	for i, arg := range call.Arguments {
		a, err := arg.eval(env)
		if err != nil {
			return nil, err
		}
		if a == nil {
			if i == 0 {
				return nil, nil
			}
			// Unlike CONCAT, CONCAT_WS skips nil arguments.
			continue
		}
		args = append(args, a)
		tt = concatSQLType(a.SQLType(), tt)

		err = ca.add(evalCollation(a), env.collationEnv)
		if err != nil {
			return nil, err
		}
	}

	tc := ca.result()
	// If we only had numbers, we instead fall back to the default
	// collation instead of using the numeric collation.
	if tc.Coercibility == collations.CoerceNumeric {
		tc = typedCoercionCollation(tt, call.collate)
	}

	var sep []byte
	var buf []byte
	for i, arg := range args {
		if i > 1 {
			buf = append(buf, sep...)
		}
		switch a := arg.(type) {
		case *evalBytes:
			var err error
			if i == 0 {
				sep, err = concatConvert(nil, a, tc)
				if err != nil {
					return nil, err
				}
				continue
			}
			buf, err = concatConvert(buf, a, tc)
			if err != nil {
				return nil, err
			}
		default:
			c, err := evalToVarchar(a, tc.Collation, true)
			if err != nil {
				return nil, err
			}
			if i == 0 {
				sep = c.bytes
				continue
			}
			buf = append(buf, c.bytes...)
		}
	}

	return newEvalRaw(tt, buf, tc), nil
}

func (call *builtinConcatWs) compile(c *compiler) (ctype, error) {
	var ca collationAggregation
	tt := sqltypes.VarChar

	var skip *jump
	args := make([]ctype, 0, len(call.Arguments)-1)
	for i, arg := range call.Arguments {
		a, err := arg.compile(c)
		if err != nil {
			return ctype{}, err
		}
		tt = concatSQLType(a.Type, tt)

		err = ca.add(a.Col, c.env.CollationEnv())
		if err != nil {
			return ctype{}, err
		}

		args = append(args, a)

		if i == 0 {
			skip = c.compileNullCheck1(a)
			continue
		}
	}

	tc := ca.result()
	// If we only had numbers, we instead fall back to the default
	// collation instead of using the numeric collation.
	if tc.Coercibility == collations.CoerceNumeric {
		tc = typedCoercionCollation(tt, call.collate)
	}

	for i, arg := range args {
		offset := len(args) - i
		var skip *jump
		if i != 0 {
			skip = c.compileNullCheckOffset(arg, offset)
		}
		switch arg.Type {
		case sqltypes.VarBinary, sqltypes.Binary, sqltypes.Blob:
			if tc.Collation != collations.CollationBinaryID {
				c.asm.Convert_xce(offset, arg.Type, tc.Collation)
			}
		case sqltypes.VarChar, sqltypes.Char, sqltypes.Text:
			fromCharset := colldata.Lookup(arg.Col.Collation).Charset()
			toCharset := colldata.Lookup(tc.Collation).Charset()
			if fromCharset != toCharset && !toCharset.IsSuperset(fromCharset) {
				c.asm.Convert_xce(offset, arg.Type, tc.Collation)
			}
		default:
			c.asm.Convert_xce(offset, arg.Type, tc.Collation)
		}
		c.asm.jumpDestination(skip)
	}

	c.asm.Fn_CONCAT_WS(tt, tc, len(args)-1)
	c.asm.jumpDestination(skip)

	return ctype{Type: tt, Flag: args[0].Flag, Col: tc}, nil
}
