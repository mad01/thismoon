import Foundation
import Yams

/// Machine-local additions to the safe-mode deny-list, read from
/// ~/.config/toss-bin/config.yaml. Extend-only: entries add protected
/// paths; nothing in the config can unblock a built-in entry.
public struct TossConfig: Equatable {
    /// Additional exact-match protected paths (the path itself, not its contents).
    public var protectedPaths: [String]
    /// Additional subtree-protected paths (the path and everything under it).
    public var protectedTrees: [String]

    public init(protectedPaths: [String] = [], protectedTrees: [String] = []) {
        self.protectedPaths = protectedPaths
        self.protectedTrees = protectedTrees
    }
}

extension TossConfig: Decodable {
    enum CodingKeys: String, CodingKey {
        case protectedPaths = "protected_paths"
        case protectedTrees = "protected_trees"
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        protectedPaths = try container.decodeIfPresent([String].self, forKey: .protectedPaths) ?? []
        protectedTrees = try container.decodeIfPresent([String].self, forKey: .protectedTrees) ?? []
    }
}

public enum ConfigLoader {
    /// $TOSS_BIN_CONFIG overrides the default location (tests pin it so a
    /// machine's real config never leaks into a run).
    public static var configPath: String {
        if let override = ProcessInfo.processInfo.environment["TOSS_BIN_CONFIG"],
            !override.isEmpty
        {
            return override
        }
        return FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent(".config/toss-bin/config.yaml").path
    }

    /// Load the config; nil when the file does not exist, an empty config
    /// when the file is empty. Throws on unreadable or malformed YAML —
    /// the caller decides how loudly to warn.
    public static func load(path: String = configPath) throws -> TossConfig? {
        guard FileManager.default.fileExists(atPath: path) else {
            return nil
        }
        let text = try String(contentsOfFile: path, encoding: .utf8)
        guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return TossConfig()
        }
        return try YAMLDecoder().decode(TossConfig.self, from: text)
    }

    /// Turn a config into validator extras: expand a leading ~, strip
    /// trailing slashes, drop empties, dedupe.
    public static func extraProtections(from config: TossConfig) -> PathSafety.ExtraProtections {
        PathSafety.ExtraProtections(
            exactPaths: normalize(config.protectedPaths),
            treePaths: normalize(config.protectedTrees)
        )
    }

    static func normalize(_ paths: [String]) -> Set<String> {
        Set(
            paths.compactMap { raw -> String? in
                var path = (raw as NSString).expandingTildeInPath
                while path.count > 1 && path.hasSuffix("/") {
                    path = String(path.dropLast())
                }
                return path.isEmpty ? nil : path
            }
        )
    }
}
