import Foundation

/// Safety validation for paths before trashing
public enum PathSafety {

    /// Protected system paths that must NEVER be trashed (exact match only).
    /// Users may still manage files *inside* these directories.
    public static let denyList: Set<String> = [
        "/",
        "/Library",
        "/Applications",
        "/Users",
        "/Volumes",
        "/etc",
        "/var",
        "/tmp",
        "/opt",
        "/Developer",
    ]

    /// System paths where the ENTIRE subtree is protected — nothing inside
    /// these should ever be trashed.
    public static let treeDenyList: Set<String> = [
        "/System",
        "/bin",
        "/sbin",
        "/usr",
        "/private",
        "/cores",
        "/.Spotlight-V100",
        "/.fseventsd",
        "/.vol",
        "/.DocumentRevisions-V100",
        "/.PKInstallSandboxManager",
        "/.PKInstallSandboxManager-SystemSoftware",
        "/.MobileBackups",
        "/.file",
    ]

    /// Subtrees exempt from tree deny — user-managed paths within
    /// otherwise-protected trees (e.g., Homebrew under /usr/local).
    public static let treeAllowList: Set<String> = [
        "/usr/local",
    ]

    /// Home directory paths that must NEVER be trashed
    public static let homeDenyList: Set<String> = [
        "",           // Home directory itself (~)
        ".Trash",     // The trash itself
        "Library",    // User's Library
        "Documents",
        "Desktop",
        "Downloads",
        "Pictures",
        "Music",
        "Movies",
        "Applications",
        "Public",
    ]

    /// Machine-local additions to the deny-list, loaded from the config
    /// file. Extend-only by construction: extras add protected paths and
    /// can never unblock a built-in entry.
    public struct ExtraProtections: Equatable {
        public static let none = ExtraProtections(exactPaths: [], treePaths: [])

        /// Additional exact-match protected paths.
        public let exactPaths: Set<String>
        /// Additional subtree-protected paths (the path and everything under it).
        public let treePaths: Set<String>

        public init(exactPaths: Set<String>, treePaths: Set<String>) {
            self.exactPaths = exactPaths
            self.treePaths = treePaths
        }
    }

    public enum ValidationError: Error, Equatable {
        case emptyPath
        case protectedSystemPath(String)
        case protectedHomePath(String)
        case pathContainsTraversal
        case homeDirectoryItself

        public var description: String {
            switch self {
            case .emptyPath:
                return "Empty path is not allowed"
            case .protectedSystemPath(let path):
                return "Protected system path: \(path)"
            case .protectedHomePath(let path):
                return "Protected home directory path: \(path)"
            case .pathContainsTraversal:
                return "Path contains directory traversal"
            case .homeDirectoryItself:
                return "Cannot trash home directory"
            }
        }
    }

    /// Validate a path is safe to trash
    /// Returns nil if safe, or ValidationError if dangerous
    public static func validate(
        path: String,
        extra: ExtraProtections = .none
    ) -> ValidationError? {
        guard !path.isEmpty else {
            return .emptyPath
        }

        let url = URL(fileURLWithPath: path)

        // Trashing a symlink moves the link itself, never its target, so
        // validate where the link lives rather than what it points at.
        // The parent chain is still fully resolved so traversal through
        // symlinked directories can't reach into protected trees.
        let fileType = (try? FileManager.default.attributesOfItem(atPath: url.path))?[.type]
            as? FileAttributeType
        let resolvedPath: String
        if fileType == .typeSymbolicLink {
            resolvedPath = url.deletingLastPathComponent()
                .resolvingSymlinksInPath()
                .appendingPathComponent(url.lastPathComponent)
                .path
        } else {
            resolvedPath = url.resolvingSymlinksInPath().path
        }

        return validateResolved(path: resolvedPath, extra: extra)
    }

    private static func validateResolved(
        path resolvedPath: String,
        extra: ExtraProtections
    ) -> ValidationError? {
        if denyList.contains(resolvedPath) || extra.exactPaths.contains(resolvedPath) {
            return .protectedSystemPath(resolvedPath)
        }

        if treeDenyList.contains(resolvedPath) || extra.treePaths.contains(resolvedPath) {
            return .protectedSystemPath(resolvedPath)
        }
        for denied in treeDenyList {
            if resolvedPath.hasPrefix(denied + "/") {
                let isExempt = treeAllowList.contains { exempt in
                    resolvedPath == exempt || resolvedPath.hasPrefix(exempt + "/")
                }
                if !isExempt {
                    return .protectedSystemPath(denied)
                }
            }
        }

        // Config-added trees take no exemptions: the treeAllowList carve-outs
        // exist for built-in system trees, not for paths a user chose to
        // protect wholesale.
        for denied in extra.treePaths {
            if resolvedPath.hasPrefix(denied + "/") {
                return .protectedSystemPath(denied)
            }
        }


        // Get home directory
        let home = FileManager.default.homeDirectoryForCurrentUser.path

        // Check if trying to trash home directory itself
        if resolvedPath == home {
            return .homeDirectoryItself
        }

        // Check protected home subdirectories
        if resolvedPath.hasPrefix(home + "/") {
            let relativePath = String(resolvedPath.dropFirst(home.count + 1))
            let topComponent = relativePath.split(separator: "/").first.map(String.init) ?? relativePath

            if homeDenyList.contains(topComponent) && !relativePath.contains("/") {
                // Trying to delete the protected directory itself, not something inside it
                return .protectedHomePath(topComponent)
            }
        }

        return nil
    }
}
