import Foundation

/// The containment check behind `--temp-only`: a path passes only when its
/// real location sits strictly inside one of the built-in temp roots.
///
/// The roots are fixed here and nothing reads them from config. An agent
/// harness may allow `toss-bin --temp-only` without asking because this
/// binary refuses everything else; a config file is something an agent
/// could edit, so it must not be able to widen the roots.
public enum TempOnly {

    /// The roots as a caller writes them, for messages.
    public static let rootNames = ["/tmp", "/var/tmp", "~/.tmp"]

    /// The roots as real paths, the form `resolve` produces. `/tmp` and
    /// `/var/tmp` are symlinks into `/private` on macOS; the literal forms
    /// stay listed for a system where they are real directories. Only the
    /// home directory is resolved, never `.tmp` itself: a `~/.tmp` that is a
    /// symlink is judged by where it points.
    public static func roots(home: String = accountHome()) -> [String] {
        let realHome = realPath(home) ?? home
        return ["/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp", realHome + "/.tmp"]
    }

    public enum Refusal: Error, Equatable {
        case emptyPath
        case dotEntry
        case unresolvable
        case tempRoot(String)
        case outsideRoots(String)

        public var description: String {
            switch self {
            case .emptyPath:
                return "empty path"
            case .dotEntry:
                return "refusing to trash '.' or '..'"
            case .unresolvable:
                return "its parent directories do not resolve to a real location"
            case .tempRoot(let resolved):
                return "\(resolved) is a temp root itself, not a path inside one"
            case .outsideRoots(let resolved):
                return "resolves to \(resolved), outside the temp roots"
            }
        }
    }

    /// Where a checked path lives: the real directory that holds it, with no
    /// symlink, `.` or `..` left in it, and the final component as written.
    /// The `--temp-only` move walks `directory` itself, never the path the
    /// caller wrote.
    public struct Location: Equatable {
        public let directory: String
        public let name: String

        public var path: String {
            TempOnly.join(directory, [name])
        }
    }

    /// Returns nil when `path` may be trashed under `--temp-only`, or the
    /// reason it may not.
    public static func check(
        path: String,
        home: String = accountHome(),
        cwd: String = FileManager.default.currentDirectoryPath
    ) -> Refusal? {
        if case .failure(let refusal) = locate(path: path, home: home, cwd: cwd) {
            return refusal
        }
        return nil
    }

    /// The real location of `path` when it sits strictly inside a temp
    /// root, or the reason it may not be trashed under `--temp-only`.
    public static func locate(
        path: String,
        home: String = accountHome(),
        cwd: String = FileManager.default.currentDirectoryPath
    ) -> Result<Location, Refusal> {
        guard !path.isEmpty else {
            return .failure(.emptyPath)
        }
        let location: Location
        switch resolve(path, home: home, cwd: cwd) {
        case .failure(let refusal):
            return .failure(refusal)
        case .success(let resolved):
            location = resolved
        }
        let resolved = location.path
        let allowed = roots(home: home)
        if allowed.contains(resolved) {
            return .failure(.tempRoot(resolved))
        }
        if allowed.contains(where: { resolved.hasPrefix($0 + "/") }) {
            return .success(location)
        }
        return .failure(.outsideRoots(resolved))
    }

    /// Expands a leading `~` to `home`, so the caller trashes the same path
    /// the check judged. `~user` forms are left as written.
    public static func expandTilde(_ path: String, home: String = accountHome()) -> String {
        if path == "~" {
            return home
        }
        if path.hasPrefix("~/") {
            return home + path.dropFirst()
        }
        return path
    }

    /// The account's home directory from the password database. `$HOME` and
    /// `CFFIXED_USER_HOME` are ignored: an environment prefix must not move
    /// the `~/.tmp` root.
    public static func accountHome() -> String {
        if let entry = getpwuid(getuid()), let dir = entry.pointee.pw_dir {
            return String(cString: dir)
        }
        return FileManager.default.homeDirectoryForCurrentUser.path
    }

    /// The real location `path` names: every parent directory resolved
    /// through `..` and symlinks, the final component kept as written, so a
    /// symlink is judged by where the link lives.
    static func resolve(_ path: String, home: String, cwd: String) -> Result<Location, Refusal> {
        let expanded = expandTilde(path, home: home)
        let absolute = expanded.hasPrefix("/") ? expanded : cwd + "/" + expanded
        var parents = absolute.split(separator: "/").map(String.init)
        guard let last = parents.popLast() else {
            return .failure(.outsideRoots("/"))
        }
        if last == "." || last == ".." {
            return .failure(.dotEntry)
        }
        guard let parent = resolveDirectory(parents) else {
            return .failure(.unresolvable)
        }
        return .success(Location(directory: parent, name: last))
    }

    /// Resolves a directory chain with realpath(3). When the chain's tail
    /// does not exist yet, the deepest existing ancestor is resolved and the
    /// missing names are appended as written, so `-f` on a path under a
    /// removed directory still gets judged. A missing tail holding `.`, `..`
    /// or a dangling symlink has no real location to judge and returns nil.
    private static func resolveDirectory(_ components: [String]) -> String? {
        var existing = components
        var missing: [String] = []
        while true {
            if let real = realPath("/" + existing.joined(separator: "/")) {
                return appendMissing(missing, to: real)
            }
            guard errno == ENOENT, let name = existing.popLast() else {
                return nil
            }
            missing.insert(name, at: 0)
        }
    }

    private static func appendMissing(_ missing: [String], to real: String) -> String? {
        guard !missing.contains(where: { $0 == "." || $0 == ".." }) else {
            return nil
        }
        // realpath reports ENOENT for a dangling symlink too; such an entry
        // exists, and its target could appear later.
        if let first = missing.first, lstatSucceeds(join(real, [first])) {
            return nil
        }
        return join(real, missing)
    }

    private static func join(_ base: String, _ names: [String]) -> String {
        guard !names.isEmpty else {
            return base
        }
        let prefix = base == "/" ? "" : base
        return prefix + "/" + names.joined(separator: "/")
    }

    private static func lstatSucceeds(_ path: String) -> Bool {
        var info = stat()
        return lstat(path, &info) == 0
    }

    static func realPath(_ path: String) -> String? {
        guard let resolved = realpath(path, nil) else {
            return nil
        }
        defer { free(resolved) }
        return String(cString: resolved)
    }
}
