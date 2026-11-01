---
id: speak-pause
title: Playback states and what Pause does
split: train
query: 'sym:Pause|Resume'
file: 'playback/engine\.go$'
anchors: [services/speak/internal/playback/engine.go]
max_files: 1
max_answer_chars: 1000
expected:
  - '(?i)\bidle\b|StateIdle'
  - '(?i)\bplaying\b|StatePlaying'
  - '(?i)\bpaused\b|StatePaused'
  - '(?i)\bstopped\b|StateStopped'
  - '(?i)\bstarting\b|StateStarting'
  - 'SIGSTOP'
  - '(?i)already paused'
  - '(?i)nothing is playing'
  - '(?i)releaseLock|releas\w*[^.\n]{0,40}lock|lock[^.\n]{0,20}releas'
forbidden:
  - 'SIGKILL'
  - 'SIGTERM'
  - '(?i)\bbuffering\b'
oracle: |
  - State (engine.go:47): idle, playing, paused, stopped, starting.
  - Pause (engine.go:234) sends SIGSTOP to the afplay child, marks the engine paused and releases the playback lock; it returns "Paused at part N of M. Playback lock released."
  - When already paused it returns "Already paused."; with no child process it returns "Nothing is playing."; a session mismatch returns the mismatch result from checkSession.
notes: >-
  A behavior question: the answer is spread over a const block and one
  method body, with exact strings the worker has to quote. The state
  patterns accept the constant name or its string value, since a worker
  that writes StateIdle has read the same line. The release pattern allows
  a qualifier between the verb and the lock, the method name releaseLock,
  and the reply text "lock released". The forbidden signals are the ones a
  model guesses when it stops reading.
---
What playback states does the engine define, what does Pause do to the child process and the lock, and which messages does Pause return when it cannot pause?
