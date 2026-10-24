"""Torque Game Engine DSO (v33) parser/disassembler for blobs embedded in an exe."""
import struct
import sys

OPS = """FUNC_DECL CREATE_OBJECT ADD_OBJECT END_OBJECT
JMPIFFNOT JMPIFNOT JMPIFF JMPIF JMPIFNOT_NP JMPIF_NP JMP RETURN
CMPEQ CMPGR CMPGE CMPLT CMPLE CMPNE
XOR MOD BITAND BITOR NOT NOTF ONESCOMPLEMENT SHR SHL AND OR
ADD SUB MUL DIV NEG
SETCURVAR SETCURVAR_CREATE SETCURVAR_ARRAY SETCURVAR_ARRAY_CREATE
LOADVAR_UINT LOADVAR_FLT LOADVAR_STR SAVEVAR_UINT SAVEVAR_FLT SAVEVAR_STR
SETCUROBJECT SETCUROBJECT_NEW SETCURFIELD SETCURFIELD_ARRAY
LOADFIELD_UINT LOADFIELD_FLT LOADFIELD_STR SAVEFIELD_UINT SAVEFIELD_FLT SAVEFIELD_STR
STR_TO_UINT STR_TO_FLT STR_TO_NONE FLT_TO_UINT FLT_TO_STR FLT_TO_NONE UINT_TO_FLT UINT_TO_STR UINT_TO_NONE
LOADIMMED_UINT LOADIMMED_FLT TAG_TO_STR LOADIMMED_STR LOADIMMED_IDENT
CALLFUNC_RESOLVE CALLFUNC
ADVANCE_STR ADVANCE_STR_APPENDCHAR ADVANCE_STR_COMMA ADVANCE_STR_NUL
REWIND_STR TERMINATE_REWIND_STR COMPARE_STR PUSH PUSH_FRAME BREAK INVALID""".split()
# Botzin's engine build has 3 extra (unused) opcodes compared to stock TGE
OPS = OPS[:-1]
OPS.insert(2, 'X1')
OPS.insert(3, 'X2')
OPS.insert(OPS.index('CALLFUNC') + 1, 'X3')
OPS.append('INVALID')
OP = {name: i for i, name in enumerate(OPS)}


class Blob:
    def __init__(self, data, off):
        self.data, self.start, self.p = data, off, off

    def u32(self):
        v = struct.unpack_from('<I', self.data, self.p)[0]
        self.p += 4
        return v

    def u8(self):
        v = self.data[self.p]
        self.p += 1
        return v

    def raw(self, n):
        v = self.data[self.p:self.p + n]
        self.p += n
        return v


def cstr(table, off):
    end = table.index(b'\0', off)
    return table[off:end].decode('latin-1')


def parse(data, off):
    b = Blob(data, off)
    version = b.u32()
    gstr = b.raw(b.u32())
    gflt = [struct.unpack_from('<d', b.raw(8))[0] for _ in range(b.u32())]
    fstr = b.raw(b.u32())
    fflt = [struct.unpack_from('<d', b.raw(8))[0] for _ in range(b.u32())]
    code_size = b.u32()
    lbp = b.u32()
    code = []
    for _ in range(code_size):
        c = b.u8()
        code.append(b.u32() if c == 0xFF else c)
    lines = [b.u32() for _ in range(lbp * 2)]
    idents = {}
    for _ in range(b.u32()):
        o = b.u32()
        name = cstr(gstr, o)
        for _ in range(b.u32()):
            idents[b.u32()] = name
    return dict(version=version, gstr=gstr, gflt=gflt, fstr=fstr, fflt=fflt,
                code=code, idents=idents, end=b.p)


def disasm(d):
    code, idents = d['code'], d['idents']
    out = []
    ip = 0
    fn_end = -1
    ident = lambda i: idents.get(i, '?%d' % code[i] if i < len(code) else '?EOF')

    def s(i):
        table = d['fstr'] if ip < fn_end else d['gstr']
        try:
            return repr(cstr(table, code[i]))
        except Exception:
            return '<bad str %d>' % code[i]

    def f(i):
        table = d['fflt'] if ip < fn_end else d['gflt']
        return table[code[i]] if code[i] < len(table) else '<bad flt>'

    while ip < len(code):
        op = code[ip]
        if op >= len(OPS):
            out.append('%5d  ??? %d' % (ip, op))
            ip += 1
            continue
        name = OPS[op]
        here = ip
        ip += 1
        args = ''
        if name == 'FUNC_DECL':
            fname, ns, pkg = ident(ip), ident(ip + 1), ident(ip + 2)
            has_body, end, argc = code[ip + 3], code[ip + 4], code[ip + 5]
            argv = [ident(ip + 6 + k) for k in range(min(argc, 20))]
            args = '%s::%s(%s) body=%d end=%d' % (ns if code[ip + 1] else '', fname, ', '.join(argv), has_body, end)
            fn_end = end
            ip += 6 + argc
            out.append('')
        elif name == 'CREATE_OBJECT':
            args = 'parent=%s datablock=%d fail=%d' % (ident(ip), code[ip + 1], code[ip + 2])
            ip += 3
        elif name in ('ADD_OBJECT', 'END_OBJECT', 'ADVANCE_STR_APPENDCHAR') or name.startswith('JMP'):
            v = code[ip]
            args = repr(chr(v)) if name == 'ADVANCE_STR_APPENDCHAR' else str(v)
            ip += 1
        elif name in ('SETCURVAR', 'SETCURVAR_CREATE', 'SETCURFIELD', 'LOADIMMED_IDENT'):
            args = ident(ip)
            ip += 1
        elif name in ('LOADIMMED_STR', 'TAG_TO_STR'):
            args = s(ip)
            ip += 1
        elif name == 'LOADIMMED_UINT':
            args = str(code[ip])
            ip += 1
        elif name == 'LOADIMMED_FLT':
            args = str(f(ip))
            ip += 1
        elif name in ('CALLFUNC', 'CALLFUNC_RESOLVE'):
            ns = ident(ip + 1) if code[ip + 1] else ''
            args = '%s%s%s type=%d' % (ns, '::' if ns else '', ident(ip), code[ip + 2])
            ip += 3
        out.append('%5d  %-22s %s' % (here, name, args))
    return out


def find_blobs(data):
    blobs = []
    p = 0
    while True:
        p = data.find(b'\x21\x00\x00\x00', p)
        if p < 0:
            return blobs
        try:
            d = parse(data, p)
            if d['code'] and d['code'][0] < len(OPS) and len(d['gstr']) > 0 and d['gstr'][-1:] == b'\0' and d['end'] - p < 2_000_000:
                blobs.append((p, d))
                p = d['end']
                continue
        except Exception:
            pass
        p += 1


if __name__ == '__main__':
    data = open(sys.argv[1], 'rb').read()
    blobs = find_blobs(data)
    needle = sys.argv[2].encode() if len(sys.argv) > 2 else None
    print('found %d blobs' % len(blobs), file=sys.stderr)
    for p, d in blobs:
        if needle and needle not in d['gstr'] and needle not in d['fstr']:
            continue
        print('=== blob at file offset 0x%x, %d code words, %d idents' % (p, len(d['code']), len(d['idents'])))
        print('\n'.join(disasm(d)))
