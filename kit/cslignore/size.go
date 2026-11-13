package cslignore

// SizeMax is the largest file, in bytes, csl's lexical index takes. csl's
// walk passes over a bigger file and hands zoekt's builder the same limit,
// so a bigger file is not in the index at all, not even by name. The value
// is zoekt's own default.
const SizeMax = 2 << 20
