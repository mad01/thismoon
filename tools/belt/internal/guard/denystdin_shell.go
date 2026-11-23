package guard

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// This file follows what a command takes on standard input from the shell
// around it, past its own redirections and the pipe in front of it: the
// redirections of each { } group and compound command it is in (`while …;
// do psql; done < f`), which every command inside reads, and what an exec
// with no command left on descriptor 0 (`exec < f; psql`), which every later
// command of the same shell reads. A ( ) body and a shell's -c string are a
// shell of their own, so an exec inside one moves nothing outside it. Nested
// code (a -c string, a substitution, eval) starts with its caller's
// standard input on descriptor 0 (`bash -c 'psql' < f`), which its own exec
// lines may replace. A call of a function the text defines gives the
// commands of its body what the call reads (`f() { psql; }; f < f.sql`).

// fdSource is what a descriptor of the shell holds once an exec with no
// command set it: redirection k of the exec at at, or, with at nil, a
// descriptor belt does not follow, named by what. from, when set, is the
// command whose standard input a nested shell starts with on the
// descriptor, which reads what that command reads (inputReader.sources).
// With none of them, it is the shell's own standard input, which belt does
// not read (`exec 3<&0`).
type fdSource struct {
	at   *inputAt
	k    int
	what string
	from *inputAt
}

// stdinWalk follows the commands of one text, in order, for where each
// reads its standard input (at): the groups open at each command and the
// descriptors an exec set so far. outer is the command whose ( ) body the
// text is, nil for none, and nested marks nested code (inputAt.nested).
// opens holds, by the index of the command that opens them, the closers of
// the groups that command opens, outermost first, and open the closers of
// the groups open now, innermost last. fns are the functions the shell
// knows.
type stdinWalk struct {
	text   string
	cmds   []shellCmd
	nested bool
	outer  *inputAt
	opens  map[int][]int
	open   []*inputAt
	fds    map[string]fdSource
	fns    *shellFuncs
}

// newStdinWalk starts the walk of cmds, read from text, with the groups
// groupsOf finds in them. The functions whose bodies are among them join
// those the ( ) the text is in knows.
func newStdinWalk(text string, cmds []shellCmd, outer *inputAt, nested bool) *stdinWalk {
	groups := groupsOf(text, cmds)
	fns := &shellFuncs{bodies: map[string][]*funcBody{}}
	if outer != nil {
		fns.parent = outer.fns
	}
	for _, closer := range slices.Sorted(maps.Keys(groups)) {
		if g := groups[closer]; g.function != "" {
			body := &funcBody{text: text, cmds: g.cmds, all: cmds, closer: closer, nested: nested}
			fns.bodies[g.function] = append(fns.bodies[g.function], body)
		}
	}
	w := &stdinWalk{text: text, cmds: cmds, nested: nested, outer: outer, fds: map[string]fdSource{}, fns: fns}
	w.opens = openersOf(groups)
	return w
}

// openersOf returns, by the index of the command that opens them, the
// closers of the groups (groupsOf), outermost first.
func openersOf(groups map[int]*shellGroup) map[int][]int {
	opens := map[int][]int{}
	for closer, g := range groups {
		opener := closer - len(g.cmds)
		opens[opener] = append(opens[opener], closer)
	}
	for _, closers := range opens {
		slices.Sort(closers)
		slices.Reverse(closers)
	}
	return opens
}

// inherit starts the walk's descriptor 0 as the standard input of caller,
// the command that runs the text as nested code (fdSource.from).
func (s *stdinWalk) inherit(caller *inputAt) {
	if caller != nil {
		s.fds["0"] = fdSource{from: caller}
	}
}

// at returns where command i, run at where, reads its standard input. The
// group a command closes ends before it, and the groups it opens start at
// it: each is read as its closer, the command its redirections stand on,
// placed where the group starts.
func (s *stdinWalk) at(i int, where runDir) *inputAt {
	for n := len(s.open); n > 0 && s.open[n-1].i <= i; n-- {
		s.open = s.open[:n-1]
	}
	stdin := s.stdin()
	for _, closer := range s.opens[i] {
		s.open = append(s.open, &inputAt{
			text: s.text, cmds: s.cmds, i: closer, runDir: where, nested: s.nested,
			outer: s.top(), body: closer - i, stdin: stdin, fns: s.fns,
		})
	}
	return &inputAt{
		text: s.text, cmds: s.cmds, i: i, runDir: where, nested: s.nested,
		outer: s.top(), stdin: stdin, fns: s.fns,
	}
}

// top returns the innermost group open now, or the ( ) the text is in.
func (s *stdinWalk) top() *inputAt {
	if n := len(s.open); n > 0 {
		return s.open[n-1]
	}
	return s.outer
}

// stdin returns what an exec, or the caller of nested code, left on
// descriptor 0, nil for nothing belt reads.
func (s *stdinWalk) stdin() *fdSource {
	if src, ok := s.fds["0"]; ok && (src.at != nil || src.what != "" || src.from != nil) {
		return &src
	}
	return nil
}

// leave takes the walk past command i, read at at: an exec with no command
// (execOnly) sets each descriptor its redirections name, left to right, for
// every later command of this shell. A file or heredoc opened for reading
// holds what the redirection gives, a copy (`<&3`) holds what the copied
// descriptor holds, and a descriptor closed or opened for writing holds
// nothing belt reads.
func (s *stdinWalk) leave(i int, at *inputAt) {
	c := s.cmds[i]
	if !execOnly(c) {
		return
	}
	for k, op := range c.redirs {
		fd := redirFd(op)
		switch op.op {
		case "<&", ">&":
			s.copyFd(fd, c.targets[k])
		case "<", "<>", "<<", "<<-", "<<<":
			s.fds[fd] = fdSource{at: at, k: k}
		default:
			delete(s.fds, fd)
		}
	}
}

// copyFd sets descriptor fd to a copy of the one target names: closed for
// -, unchanged for itself, and what the copied one holds, descriptor 0 the
// shell's own standard input until an exec sets it. A copy of any other
// descriptor the walk has not seen set is one belt does not follow on
// descriptor 0, and the shell's own elsewhere.
func (s *stdinWalk) copyFd(fd string, target placedWord) {
	v, ok := target.literal()
	src, set := s.fds[v]
	switch {
	case ok && v == "-":
		delete(s.fds, fd)
	case ok && v == fd:
	case ok && set:
		s.fds[fd] = src
	case ok && v == "0":
		s.fds[fd] = fdSource{}
	case fd == "0":
		s.fds[fd] = fdSource{what: "descriptor " + target.raw}
	default:
		delete(s.fds, fd)
	}
}

// execOnly reports whether c is an exec that runs no command (`exec < f`,
// `exec 3< f`), whose redirections stay on the shell for the commands after
// it.
func execOnly(c shellCmd) bool {
	tokens := c.tokens()
	return len(c.redirs) > 0 && slices.Contains(tokens, "exec") && len(commandRun(tokens)) == 0
}

// redirFd returns the descriptor a redirection opens: the one written in
// front of it, else 0 for an input operator and 1 for an output one.
func redirFd(op redirOp) string {
	switch {
	case op.fd != "":
		return op.fd
	case strings.HasPrefix(op.op, "<"):
		return "0"
	}
	return "1"
}

// maxFunctionReads bounds the commands of function bodies one call's reads
// follow (callHit), so a text that calls its functions many times cannot
// hold the hook past its timeout. Past it, a call of a function the shell
// knows reads as unread.
const maxFunctionReads = 4096

// shellFuncs are the functions a shell knows, by name: the bodies its text
// defines (shellGroup.function), and those the ( ) the text is in knows
// (parent). A text read as nested code, such as a -c string, is a shell of
// its own and knows none of its caller's. Like shellNames they read text,
// not shell scope: a body counts wherever the text defines it.
type shellFuncs struct {
	bodies map[string][]*funcBody
	parent *shellFuncs
}

// funcBody is one function's body, read from text: its commands, from the
// one after its { up to the one before its } (shellGroup.cmds), and in all,
// the commands of the text, the index of that } (closer), whose
// redirections every call opens. nested marks nested code
// (inputAt.nested). opens is openersOf for the body, read at its first
// call.
type funcBody struct {
	text   string
	cmds   []shellCmd
	all    []shellCmd
	closer int
	nested bool
	opens  map[int][]int
}

// named returns the bodies of the functions the shell knows by name, its
// own first.
func (f *shellFuncs) named(name string) []*funcBody {
	var out []*funcBody
	for ; f != nil; f = f.parent {
		out = append(out, f.bodies[name]...)
	}
	return out
}

// callHit reads a call of a function the shell knows, run being the call's
// words: each command of the body is read as check reads one, where the
// call runs, in a walk of its own (stdinWalk) that follows the body's groups
// and exec lines, with the body's } and then the call around them, so each
// command reads what the call reads. A definition is no call, and a call
// maxNameDepth calls deep is not read, as hiddenRun reads no deeper.
func (r *inputReader) callHit(at inputAt, run []string, patterns []deny, tools map[string]bool, depth int) *inputHit {
	c := at.cmds[at.i]
	if len(run) == 0 || depth >= maxNameDepth || definesFunction(at.text, c, c.tokens()) {
		return nil
	}
	name := run[0]
	for _, body := range at.fns.named(name) {
		if body.opens == nil {
			body.opens = openersOf(groupsOf(body.text, body.cmds))
		}
		closer := &inputAt{
			text: body.text, cmds: body.all, i: body.closer, runDir: at.runDir, nested: body.nested,
			outer: &at, fns: at.fns,
		}
		walk := &stdinWalk{
			text: body.text, cmds: body.cmds, nested: body.nested, outer: closer,
			opens: body.opens, fds: map[string]fdSource{}, fns: at.fns,
		}
		for k, bc := range body.cmds {
			if r.calls--; r.calls < 0 {
				return overCalls(name, patterns, tools)
			}
			in := walk.at(k, at.runDir)
			if hit := r.checkCall(*in, commandRun(bc.tokens()), patterns, tools, depth+1); hit != nil {
				hit.src.what += fmt.Sprintf(" (through the function %s)", name)
				return hit
			}
			walk.leave(k, in)
		}
	}
	return nil
}

// overCalls is the hit for a call of the function name once the reads have
// followed maxFunctionReads commands of function bodies: belt cannot tell
// what the body reads, so the first tool of tools, by name, reads input belt
// cannot read.
func overCalls(name string, patterns []deny, tools map[string]bool) *inputHit {
	tool := slices.Min(slices.Collect(maps.Keys(tools)))
	src := inputSource{
		what:   "the call of the function " + name,
		unread: "the call follows more commands of function bodies than belt reads",
	}
	return &inputHit{d: firstPhrase(patterns, tool), src: src, tool: tool}
}
