import Foundation

/// The move behind `--temp-only`, built so the directory that moves an entry
/// is the directory the containment check approved.
///
/// A move by path string makes the kernel resolve the parent directories
/// again, so any process that can write /tmp could swap one for a symlink
/// between the check and the move. Here the move walks the checked real
/// directory from `/` one component at a time with
/// openat(O_NOFOLLOW | O_DIRECTORY), so every component, the root's own
/// included, must still be a real directory. It then renames the entry from
/// that open directory into the open trash directory with RENAME_EXCL, so a
/// name clash never overwrites. The final component is never followed: a
/// symlink moves as a link. The trash directory is reached the same way
/// (openTrash), so a link planted on the way to it cannot redirect the move.
public enum TempOnlyMove {

    public enum Failure: Error, Equatable {
        /// The entry or one of its parent directories does not exist.
        case missing
        /// A directory on the checked path is a symlink at move time.
        case symlinkedParent(String)
        /// A directory on the way to the trash, below the home, is a symlink.
        case symlinkedTrash(String)
        /// A name on the way to the trash, below the home, is not a directory.
        case trashNotDirectory(String)
        /// The entry sits on another volume than the trash.
        case otherVolume
        case system(String, Int32)

        public var description: String {
            switch self {
            case .missing:
                return "No such file or directory"
            case .symlinkedParent(let name):
                return "\(name), a directory on the checked path, is now a symlink"
            case .symlinkedTrash(let name):
                return "\(name), on the way to the trash, is a symlink, and --temp-only never follows one there"
            case .trashNotDirectory(let name):
                return "\(name), on the way to the trash, is not a directory"
            case .otherVolume:
                return "it sits on another volume than the trash, and --temp-only never copies"
            case .system(let operation, let code):
                return "\(operation): \(String(cString: strerror(code)))"
            }
        }
    }

    /// An open directory file descriptor, closed with the last reference.
    public final class Directory {
        let fd: Int32

        init(fd: Int32) {
            self.fd = fd
        }

        deinit {
            Darwin.close(fd)
        }
    }

    /// An entry ready to move: the open directory holding it and its name.
    public struct Entry {
        public let parent: Directory
        public let name: String
        public let isDirectory: Bool
    }

    /// The directories below the home that hold the trash, in walk order:
    /// `.Trash/my-trash/<date>`.
    public static func trashComponents(date: String) -> [String] {
        [".Trash", "my-trash", date]
    }

    /// Opens `<home>/.Trash/my-trash/<date>`, the directory the entries move
    /// into, creating what is missing.
    ///
    /// The home is the walk's trusted start. It may sit behind a symlink on
    /// some machines, so it is resolved once with realpath(3) and walked
    /// from `/` like a checked directory. Below it each component is created
    /// with mkdirat, which never follows a link, and opened with
    /// openat(O_NOFOLLOW | O_DIRECTORY). A symlink at `.Trash`, `my-trash` or
    /// the dated directory is refused: followed, it would let the run create
    /// the entry in whatever directory the link names.
    public static func openTrash(home: String, date: String) -> Result<Directory, Failure> {
        guard let realHome = TempOnly.realPath(home) else {
            return .failure(.system("resolve home \(home)", errno))
        }
        var current: Directory
        switch walk(realHome) {
        case .failure(let failure):
            return .failure(failure)
        case .success(let directory):
            current = directory
        }
        for name in trashComponents(date: date) {
            switch openTrashComponent(name, in: current) {
            case .failure(let failure):
                return .failure(failure)
            case .success(let directory):
                current = directory
            }
        }
        return .success(current)
    }

    private static func openTrashComponent(_ name: String, in parent: Directory) -> Result<Directory, Failure> {
        let mode: mode_t = 0o777  // narrowed by the umask, as FileManager does
        if mkdirat(parent.fd, name, mode) != 0 {
            let code = errno
            guard code == EEXIST else {
                return .failure(.system("mkdir \(name)", code))
            }
        }
        let fd = openat(parent.fd, name, O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
        guard fd >= 0 else {
            let code = errno
            guard code == ENOTDIR || code == ELOOP else {
                return .failure(.system("open \(name)", code))
            }
            return .failure(isSymlink(name, in: parent) ? .symlinkedTrash(name) : .trashNotDirectory(name))
        }
        return .success(Directory(fd: fd))
    }

    /// Walks to the directory holding `location` and stats the entry there
    /// without following it.
    public static func prepare(_ location: TempOnly.Location) -> Result<Entry, Failure> {
        let name = location.name
        guard !name.isEmpty, name != ".", name != "..", !name.contains("/") else {
            return .failure(.system("bad entry name \(name)", EINVAL))
        }
        let parent: Directory
        switch walk(location.directory) {
        case .failure(let failure):
            return .failure(failure)
        case .success(let directory):
            parent = directory
        }
        var info = stat()
        guard fstatat(parent.fd, name, &info, AT_SYMLINK_NOFOLLOW) == 0 else {
            let code = errno
            return .failure(code == ENOENT || code == ENOTDIR ? .missing : .system("stat \(name)", code))
        }
        let isDirectory = info.st_mode & S_IFMT == S_IFDIR
        return .success(Entry(parent: parent, name: name, isDirectory: isDirectory))
    }

    /// Renames `entry` into `trash` under the first free name TrashName
    /// gives and returns that name.
    public static func move(_ entry: Entry, into trash: Directory) -> Result<String, Failure> {
        var attempt = 0
        while true {
            let candidate = TrashName.candidate(entry.name, attempt: attempt)
            if renameatx_np(entry.parent.fd, entry.name, trash.fd, candidate, UInt32(RENAME_EXCL)) == 0 {
                return .success(candidate)
            }
            let code = errno
            switch code {
            case EEXIST:
                attempt += 1
            case EXDEV:
                return .failure(.otherVolume)
            default:
                return .failure(.system("rename \(entry.name)", code))
            }
        }
    }

    /// Opens `path` from `/`, refusing any component that is not a real
    /// directory at this moment. The path comes from TempOnly.locate, so it
    /// holds no symlink, `.` or `..` unless one appeared after the check.
    static func walk(_ path: String) -> Result<Directory, Failure> {
        let rootFD = Darwin.open("/", O_RDONLY | O_DIRECTORY | O_CLOEXEC)
        guard rootFD >= 0 else {
            return .failure(.system("open /", errno))
        }
        var current = Directory(fd: rootFD)
        for name in path.split(separator: "/").map(String.init) {
            guard name != ".", name != ".." else {
                return .failure(.system("bad directory name \(name)", EINVAL))
            }
            let fd = openat(current.fd, name, O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
            guard fd >= 0 else {
                return .failure(walkFailure(errno, name: name, in: current))
            }
            current = Directory(fd: fd)
        }
        return .success(current)
    }

    /// O_NOFOLLOW with O_DIRECTORY reports a symlink as ENOTDIR, the same as
    /// a regular file, so the component is stat'ed to tell them apart. A
    /// regular file as a parent reads as a missing entry, as it does for rm.
    private static func walkFailure(_ code: Int32, name: String, in parent: Directory) -> Failure {
        switch code {
        case ENOENT:
            return .missing
        case ENOTDIR, ELOOP:
            return isSymlink(name, in: parent) ? .symlinkedParent(name) : .missing
        default:
            return .system("open \(name)", code)
        }
    }

    private static func isSymlink(_ name: String, in parent: Directory) -> Bool {
        var info = stat()
        return fstatat(parent.fd, name, &info, AT_SYMLINK_NOFOLLOW) == 0 && info.st_mode & S_IFMT == S_IFLNK
    }
}

/// The name a trashed entry takes in the dated trash directory: its own name
/// first, then `name.1.ext`, `name.2.ext`, and so on after a clash.
public enum TrashName {
    public static func candidate(_ name: String, attempt: Int) -> String {
        guard attempt > 0 else {
            return name
        }
        let url = URL(fileURLWithPath: "/" + name)
        let baseName = url.deletingPathExtension().lastPathComponent
        let ext = url.pathExtension
        return ext.isEmpty ? "\(baseName).\(attempt)" : "\(baseName).\(attempt).\(ext)"
    }
}
