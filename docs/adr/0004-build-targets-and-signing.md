# Build targets and artifact signing

Release artifacts build for darwin/arm64, linux/amd64, and linux/arm64 only. No Windows targets anywhere — nothing in the platform runs there, and carrying a fourth target means testing surface without users. Artifacts ship with a checksums.txt and cosign keyless signatures (GitHub OIDC identity); local from-checkout installs keep the "mad01 Local Signing" codesign identity so macOS keeps trusting rebuilt binaries.
