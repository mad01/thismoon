import Foundation
import TossBinCore

let VERSION = "1.3.1" // x-release-please-version

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

/// Today as the dated trash directory names it, ISO 8601: YYYY-MM-DD.
func todayDate() -> String {
    let dateFormatter = DateFormatter()
    dateFormatter.dateFormat = "yyyy-MM-dd"
    return dateFormatter.string(from: Date())
}

/// ~/.Trash/my-trash/YYYY-MM-DD/ for `date`, today by default. Call after
/// CLI.revertSudo() so the home is the user's.
func todayTrashDirectory(date: String = todayDate()) -> URL {
    TempOnlyMove.trashComponents(date: date).reduce(FileManager.default.homeDirectoryForCurrentUser) {
        $0.appendingPathComponent($1)
    }
}

/// Move files to ~/.Trash/my-trash/YYYY-MM-DD/
func toss(_ paths: [String], safeMode: Bool, dryRun: Bool, recursive: Bool, force: Bool, removeDir: Bool, extra: PathSafety.ExtraProtections = .none) {
    // Revert sudo to use user's trash
    CLI.revertSudo()

    let fileManager = FileManager.default
    let todayTrash = todayTrashDirectory()

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
        var attempt = 0

        // fileExists follows symlinks, so a broken symlink already in the
        // trash would read as absent and the move would collide. Stat the
        // entry itself instead.
        func entryExists(_ path: String) -> Bool {
            (try? fileManager.attributesOfItem(atPath: path)) != nil
        }

        while entryExists(destination.path) {
            attempt += 1
            let newName = TrashName.candidate(url.lastPathComponent, attempt: attempt)
            destination = todayTrash.appendingPathComponent(newName)
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

struct Blocked: Error {
    let reason: String
}

/// A path that passed --temp-only, with the real location the check
/// approved. The move works on `location`, never on `path`.
struct ApprovedPath {
    let path: String
    let location: TempOnly.Location
}

/// Under --temp-only: the real location `path` may be trashed from, or why
/// not (the containment check first, then the deny-list).
func approveTempOnly(_ path: String, extra: PathSafety.ExtraProtections) -> Result<ApprovedPath, Blocked> {
    switch TempOnly.locate(path: path) {
    case .failure(let refusal):
        return .failure(Blocked(reason: "--temp-only: \(refusal.description)"))
    case .success(let location):
        if let error = PathSafety.validate(path: path, extra: extra) {
            return .failure(Blocked(reason: error.description))
        }
        return .success(ApprovedPath(path: path, location: location))
    }
}

/// Why a path may not be trashed: the --temp-only containment check when
/// that mode is on, then the deny-list. Nil when the path passes.
func blockReason(_ path: String, extra: PathSafety.ExtraProtections, tempOnly: Bool) -> String? {
    guard tempOnly else {
        return PathSafety.validate(path: path, extra: extra)?.description
    }
    if case .failure(let blocked) = approveTempOnly(path, extra: extra) {
        return blocked.reason
    }
    return nil
}

/// The line that names the allowed roots after a --temp-only refusal.
func tempRootsNote() -> String {
    "toss-bin: --temp-only takes only paths strictly inside "
        + TempOnly.roots().joined(separator: ", ")
}

/// Validate paths only - no file operations
func validatePaths(
    _ paths: [String],
    extra: PathSafety.ExtraProtections = .none,
    tempOnly: Bool = false
) -> Int32 {
    var hasErrors = false
    for path in paths {
        if let reason = blockReason(path, extra: extra, tempOnly: tempOnly) {
            print("BLOCKED: \(path) - \(reason)")
            hasErrors = true
        } else {
            print("OK: \(path)")
        }
    }
    if tempOnly && hasErrors {
        print(tempRootsNote(), to: .standardError)
    }
    return hasErrors ? 1 : 0
}

/// --temp-only is all-or-nothing: when any path fails the containment check
/// or the deny-list, the run trashes nothing and exits 1. Otherwise returns
/// every path with the location the check approved.
func refuseUnlessAllPass(_ paths: [String], extra: PathSafety.ExtraProtections) -> [ApprovedPath] {
    var approved: [ApprovedPath] = []
    var refused = false
    for path in paths {
        switch approveTempOnly(path, extra: extra) {
        case .failure(let blocked):
            print("toss-bin: REFUSED: \(path): \(blocked.reason)", to: .standardError)
            refused = true
        case .success(let passed):
            approved.append(passed)
        }
    }
    guard refused else {
        return approved
    }
    print(tempRootsNote() + "; nothing was trashed", to: .standardError)
    exit(1)
}

/// Opens the dated trash directory for the --temp-only move, creating what
/// is missing without following a symlink below the home
/// (TempOnlyMove.openTrash). It runs before the first move, so when it
/// cannot, nothing has been trashed and the run exits 1.
func openTodayTrash(date: String, shownAs todayTrash: URL) -> TempOnlyMove.Directory {
    let home = FileManager.default.homeDirectoryForCurrentUser.path
    switch TempOnlyMove.openTrash(home: home, date: date) {
    case .failure(let failure):
        print(
            "toss-bin: REFUSED: cannot open the trash directory \(todayTrash.path): "
                + "\(failure.description); nothing was trashed",
            to: .standardError
        )
        exit(1)
    case .success(let trash):
        return trash
    }
}

/// The --temp-only move. Each entry moves from the real directory the check
/// approved, walked by file descriptor (TempOnlyMove), so swapping a parent
/// directory for a symlink mid-run cannot redirect it. The flags behave as
/// in toss().
func tossTempOnly(_ approved: [ApprovedPath], dryRun: Bool, recursive: Bool, force: Bool, removeDir: Bool) {
    CLI.revertSudo()
    let today = todayDate()
    let todayTrash = todayTrashDirectory(date: today)
    var trash: TempOnlyMove.Directory?
    var hasErrors = false

    for item in approved {
        let location = item.location
        let entry: TempOnlyMove.Entry
        switch TempOnlyMove.prepare(location) {
        case .failure(.missing):
            if !force {
                print("toss-bin: \(location.name): No such file or directory", to: .standardError)
                hasErrors = true
            }
            continue
        case .failure(let failure):
            print("toss-bin: REFUSED: \(item.path): \(failure.description)", to: .standardError)
            hasErrors = true
            continue
        case .success(let prepared):
            entry = prepared
        }

        if entry.isDirectory && !recursive && !removeDir {
            print("toss-bin: \(location.name): is a directory", to: .standardError)
            hasErrors = true
            continue
        }

        if dryRun {
            let destination = todayTrash.appendingPathComponent(location.name)
            print("[dry-run] [safe-mode: OK] \(location.path) -> \(destination.path)")
            continue
        }

        let into = trash ?? openTodayTrash(date: today, shownAs: todayTrash)
        trash = into
        switch TempOnlyMove.move(entry, into: into) {
        case .success(let name):
            print("\(location.path) -> \(todayTrash.appendingPathComponent(name).path)", to: .standardError)
        case .failure(let failure):
            print("toss-bin: Failed to trash \(location.name): \(failure.description)", to: .standardError)
            hasErrors = true
        }
    }

    if hasErrors {
        exit(1)
    }
}

func printUsage() {
    print("Usage: toss-bin [--help | -h] [--version | -v] [--safe-mode] [--temp-only] [--dry-run] [--validate] <path> [...]")
    print("")
    print("Move files to ~/.Trash/my-trash/YYYY-MM-DD/")
    print("")
    print("Options:")
    print("  -r, -R, --recursive  Allow trashing directories (recursively)")
    print("  -d                   Allow trashing directories")
    print("  -f                   Ignore nonexistent files (no error)")
    print("  --safe-mode          Block trashing protected system paths")
    print("  --temp-only          Refuse the whole run unless every path sits inside")
    print("                       /tmp, /var/tmp or ~/.tmp (implies --safe-mode)")
    print("  --dry-run            Show what would be done without doing it")
    print("  --validate           Only check if paths would be blocked (no file operations)")
    print("")
    print("Subcommands (sole argument only):")
    print("  docs                 Print the embedded operating doc (runtime debugging guide)")
    print("")
    print("-h, -v, and docs act as help/version/docs only when they are the sole")
    print("argument; mixed with paths they are treated as ordinary rm arguments.")
    print("A file or directory named docs in the working directory takes priority:")
    print("sole `docs` then trashes it instead of printing the doc.")
}

// Main
guard !CLI.arguments.isEmpty else {
    print("Usage: toss-bin <path> [...]", to: .standardError)
    exit(1)
}

let parsed = parseArguments(CLI.arguments)

// `docs` only counts as a subcommand when it is the sole argument AND no
// entry named "docs" exists in the working directory — unlike -h/-v, "docs"
// is a plausible filename, and printing the doc while leaving the file in
// place would be a silent no-op for a caller trying to trash it. The stat
// deliberately does not follow symlinks, matching how toss() decides
// existence. (The rm alias always adds --safe-mode and never matches the
// sole-argument form anyway.)
if CLI.arguments == ["docs"],
    (try? FileManager.default.attributesOfItem(atPath: "docs")) == nil {
    print(operatingDoc)
    exit(0)
}

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

// --temp-only holds wherever it appears before `--`, implies --safe-mode,
// and no flag turns either back off.
let tempOnly = parsed.longFlags.contains("--temp-only")
let safeMode = tempOnly || parsed.longFlags.contains("--safe-mode")
let dryRun = parsed.longFlags.contains("--dry-run")
let validateOnly = parsed.longFlags.contains("--validate")
let recursive = parsed.flags.contains("r")
    || parsed.flags.contains("R")
    || parsed.longFlags.contains("--recursive")
let force = parsed.flags.contains("f")
let removeDir = parsed.flags.contains("d")

// Under --temp-only a leading ~ is expanded here, so the deny-list and the
// messages see the same path the containment check judged.
let paths = tempOnly ? parsed.paths.map { TempOnly.expandTilde($0) } : parsed.paths

if validateOnly {
    guard !paths.isEmpty else {
        print("Usage: toss-bin --validate <path> [...]", to: .standardError)
        exit(1)
    }
    exit(validatePaths(paths, extra: loadExtraProtections(), tempOnly: tempOnly))
}

guard !paths.isEmpty else {
    exit(0)
}

let extra = safeMode ? loadExtraProtections() : .none
if tempOnly {
    tossTempOnly(
        refuseUnlessAllPass(paths, extra: extra),
        dryRun: dryRun,
        recursive: recursive,
        force: force,
        removeDir: removeDir
    )
    exit(0)
}

toss(
    paths,
    safeMode: safeMode,
    dryRun: dryRun,
    recursive: recursive,
    force: force,
    removeDir: removeDir,
    extra: extra
)
