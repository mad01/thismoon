import Foundation
import TossBinCore

let VERSION = "1.1.0" // x-release-please-version

/// Load deny-list additions from the config file. A malformed config warns
/// and falls back to the built-in list so rm keeps working — the default
/// safety net is never weakened by a broken config.
func loadExtraProtections() -> PathSafety.ExtraProtections {
    do {
        guard let config = try ConfigLoader.load() else {
            return .none
        }
        return ConfigLoader.extraProtections(from: config)
    } catch {
        print(
            "toss-bin: warning: ignoring \(ConfigLoader.configPath): \(error)",
            to: .standardError
        )
        return .none
    }
}

/// Move files to ~/.Trash/my-trash/YYYY-MM-DD/
func toss(_ paths: [String], safeMode: Bool, dryRun: Bool, recursive: Bool, force: Bool, removeDir: Bool, extra: PathSafety.ExtraProtections = .none) {
    // Revert sudo to use user's trash
    CLI.revertSudo()

    let fileManager = FileManager.default
    let trashBase = fileManager.homeDirectoryForCurrentUser
        .appendingPathComponent(".Trash")
        .appendingPathComponent("my-trash")

    // ISO 8601 date format: YYYY-MM-DD
    let dateFormatter = DateFormatter()
    dateFormatter.dateFormat = "yyyy-MM-dd"
    let today = dateFormatter.string(from: Date())

    let todayTrash = trashBase.appendingPathComponent(today)

    var directoryCreated = false
    var hasErrors = false

    for path in paths {
        // rm refuses "." and ".." — trashing them would move the shell's
        // own working directory (or its parent) out from under it.
        let lastComponent = path.split(separator: "/").last.map(String.init) ?? path
        if lastComponent == "." || lastComponent == ".." {
            print("toss-bin: \(path): refusing to trash '.' or '..' directory", to: .standardError)
            hasErrors = true
            continue
        }

        let url = URL(fileURLWithPath: path)

        if safeMode {
            if let error = PathSafety.validate(path: url.path, extra: extra) {
                print("toss-bin: BLOCKED: \(error.description)", to: .standardError)
                hasErrors = true
                continue
            }
        }

        // attributesOfItem does not follow a final symlink, so a broken
        // symlink counts as existing and a symlink to a directory is moved
        // as a link, not treated as a directory.
        let fileType = (try? fileManager.attributesOfItem(atPath: url.path))?[.type]
            as? FileAttributeType

        guard fileType != nil else {
            if !force {
                print("toss-bin: \(url.lastPathComponent): No such file or directory", to: .standardError)
                hasErrors = true
            }
            continue
        }

        if fileType == .typeDirectory && !recursive && !removeDir {
            print("toss-bin: \(url.lastPathComponent): is a directory", to: .standardError)
            hasErrors = true
            continue
        }

        if dryRun {
            let destination = todayTrash.appendingPathComponent(url.lastPathComponent)
            if safeMode {
                print("[dry-run] [safe-mode: OK] \(url.path) -> \(destination.path)")
            } else {
                print("[dry-run] \(url.path) -> \(destination.path)")
            }
            continue
        }

        if !directoryCreated {
            do {
                try fileManager.createDirectory(at: todayTrash, withIntermediateDirectories: true)
                directoryCreated = true
            } catch {
                print("toss-bin: Failed to create trash directory: \(error.localizedDescription)", to: .standardError)
                exit(1)
            }
        }

        var destination = todayTrash.appendingPathComponent(url.lastPathComponent)
        var counter = 1
        let baseName = url.deletingPathExtension().lastPathComponent
        let ext = url.pathExtension

        // fileExists follows symlinks, so a broken symlink already in the
        // trash would read as absent and the move would collide. Stat the
        // entry itself instead.
        func entryExists(_ path: String) -> Bool {
            (try? fileManager.attributesOfItem(atPath: path)) != nil
        }

        while entryExists(destination.path) {
            let newName = ext.isEmpty ? "\(baseName).\(counter)" : "\(baseName).\(counter).\(ext)"
            destination = todayTrash.appendingPathComponent(newName)
            counter += 1
        }

        do {
            try fileManager.moveItem(at: url, to: destination)
            print("\(url.path) -> \(destination.path)", to: .standardError)
        } catch {
            print("toss-bin: Failed to trash \(url.lastPathComponent): \(error.localizedDescription)", to: .standardError)
            hasErrors = true
        }
    }

    if hasErrors {
        exit(1)
    }
}

/// Parse arguments into flags and paths, expanding combined short flags (e.g. `-rf` -> `-r`, `-f`)
func parseArguments(_ arguments: [String]) -> (flags: Set<Character>, longFlags: Set<String>, paths: [String]) {
    var flags = Set<Character>()
    var longFlags = Set<String>()
    var paths = [String]()
    var seenDoubleDash = false

    for arg in arguments {
        if seenDoubleDash {
            paths.append(arg)
        } else if arg == "--" {
            seenDoubleDash = true
        } else if arg.hasPrefix("--") {
            longFlags.insert(arg)
        } else if arg.hasPrefix("-") && arg.count > 1 {
            // Expand combined short flags: -rf -> r, f
            for char in arg.dropFirst() {
                flags.insert(char)
            }
        } else {
            paths.append(arg)
        }
    }

    return (flags, longFlags, paths)
}

/// Validate paths only - no file operations
func validatePaths(_ paths: [String], extra: PathSafety.ExtraProtections = .none) -> Int32 {
    var hasErrors = false
    for path in paths {
        if let error = PathSafety.validate(path: path, extra: extra) {
            print("BLOCKED: \(path) - \(error.description)")
            hasErrors = true
        } else {
            print("OK: \(path)")
        }
    }
    return hasErrors ? 1 : 0
}

func printUsage() {
    print("Usage: toss-bin [--help | -h] [--version | -v] [--safe-mode] [--dry-run] [--validate] <path> [...]")
    print("")
    print("Move files to ~/.Trash/my-trash/YYYY-MM-DD/")
    print("")
    print("Options:")
    print("  -r, -R, --recursive  Allow trashing directories (recursively)")
    print("  -d                   Allow trashing directories")
    print("  -f                   Ignore nonexistent files (no error)")
    print("  --safe-mode          Block trashing protected system paths")
    print("  --dry-run            Show what would be done without doing it")
    print("  --validate           Only check if paths would be blocked (no file operations)")
    print("")
    print("-h and -v act as help/version only when they are the sole argument;")
    print("mixed with paths they are ignored like other unknown rm flags.")
}

// Main
guard !CLI.arguments.isEmpty else {
    print("Usage: toss-bin <path> [...]", to: .standardError)
    exit(1)
}

let parsed = parseArguments(CLI.arguments)

// Short -h/-v only count when they are the sole argument, so the rm alias
// can pass rm's own flags (like -v, verbose) through without toss-bin
// swallowing the file operation.
if parsed.longFlags.contains("--help") || CLI.arguments == ["-h"] {
    printUsage()
    exit(0)
}
if parsed.longFlags.contains("--version") || CLI.arguments == ["-v"] {
    print(VERSION)
    exit(0)
}

let safeMode = parsed.longFlags.contains("--safe-mode")
let dryRun = parsed.longFlags.contains("--dry-run")
let validateOnly = parsed.longFlags.contains("--validate")
let recursive = parsed.flags.contains("r")
    || parsed.flags.contains("R")
    || parsed.longFlags.contains("--recursive")
let force = parsed.flags.contains("f")
let removeDir = parsed.flags.contains("d")

if validateOnly {
    guard !parsed.paths.isEmpty else {
        print("Usage: toss-bin --validate <path> [...]", to: .standardError)
        exit(1)
    }
    exit(validatePaths(parsed.paths, extra: loadExtraProtections()))
}

guard !parsed.paths.isEmpty else {
    exit(0)
}

toss(
    parsed.paths,
    safeMode: safeMode,
    dryRun: dryRun,
    recursive: recursive,
    force: force,
    removeDir: removeDir,
    extra: safeMode ? loadExtraProtections() : .none
)
