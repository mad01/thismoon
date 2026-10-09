---
id: ai-debugging-story
label: likely_ai
bucket: ai
source: generated
license: generated for this eval
generator: claude-opus-5-5 via claude -p, 2026-10-08
words: 255
notes: First-person narrative with concrete numbers and a punchline ending; a hard AI case because it avoids most surface tells. Prompt was Write a first-person story, about 250 words, about debugging a memory leak in a Node.js service that only showed up in production. Output only the story, nothing else.
---
The graphs looked fine for six hours after every deploy. Then the heap would start climbing, slowly, about forty megabytes an hour, until the orchestrator killed the pod around three in the morning and paged me.

On my laptop the service sat at a flat 180 megabytes no matter how hard I hit it. I replayed a day of production traffic against staging and got nothing. I started to suspect the hardware.

So I took heap snapshots in production, which I'd been avoiding. I attached the inspector to one pod behind a feature flag and took a snapshot at hour one and another at hour seven. Then I diffed them over coffee that had gone cold.

The growth was all closures, thousands of them, each holding a reference to a request object. They traced back to a retry helper someone had written two years earlier. On failure it registered a listener on a shared EventEmitter so it would know when the upstream recovered. It never removed the listener.

Locally the upstream never failed, so the listener was never added. In production a flaky partner API timed out a few hundred times an hour. Each timeout left a closure holding the whole request, headers and body included, attached to an emitter that lived as long as the process.

Node had been warning us the whole time. `MaxListenersExceededWarning` appeared in the logs every morning, and we had filtered it out as noise.

The fix was one line: `once` instead of `on`. I also removed the log filter.
