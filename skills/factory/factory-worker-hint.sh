#!/bin/sh
# belt custom hint factory-worker, run on subagent-start. Experimental.
#
# Prints the factory worker brief when the agent type starts with one of the
# three worker definition names, and nothing for any other subagent. A named
# worker reports factory-worker-<tier>-<slug>; an unnamed one reports the bare
# definition name. The brief text is read from the installed skill
# (worker-brief.md), so script and brief never drift. The belt config also
# gates the hint with agent_types globs; an older belt ignores that key, so
# this filter stays and keeps the brief away from every other subagent.
case "$(jq -r '.agent_type // ""')" in
factory-worker-low* | factory-worker-medium* | factory-worker-high*)
    awk '/^```text$/{f=1;next} /^```$/{f=0} f' ~/.claude/skills/factory/worker-brief.md
    ;;
esac
