# Fail-closed staged scanning

A staged file that cannot be read from the git index (`git show :<path>` fails) blocks the commit with an error — it is never skipped with a warning. The file the scanner fails to read is exactly the file that could carry a secret unchecked, so completeness wins over availability. The explicit, auditable override is `git commit --no-verify`; silent skipping would weaken the guarantee every installed hook relies on without anyone noticing.
