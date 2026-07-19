import Foundation

extension FileHandle: @retroactive TextOutputStream {
    public func write(_ string: String) {
        write(string.data(using: .utf8)!)
    }
}

enum CLI {
    static var standardInput = FileHandle.standardInput
    static var standardOutput = FileHandle.standardOutput
    static var standardError = FileHandle.standardError

    static let arguments = Array(CommandLine.arguments.dropFirst(1))

    /// Revert back to original user if sudo
    static func revertSudo() {
        guard
            let sudoUIDstring = ProcessInfo.processInfo.environment["SUDO_UID"],
            let sudoUID = uid_t(sudoUIDstring)
        else {
            return
        }
        setuid(sudoUID)
    }
}

enum PrintOutputTarget {
    case standardOutput
    case standardError
}

private func print(
    _ items: [Any],
    separator: String = " ",
    terminator: String = "\n",
    to output: inout some TextOutputStream
) {
    let item = items.map { "\($0)" }.joined(separator: separator)
    Swift.print(item, terminator: terminator, to: &output)
}

func print(
    _ items: Any...,
    separator: String = " ",
    terminator: String = "\n",
    to output: PrintOutputTarget = .standardOutput
) {
    switch output {
    case .standardOutput:
        print(items, separator: separator, terminator: terminator)
    case .standardError:
        print(items, separator: separator, terminator: terminator, to: &CLI.standardError)
    }
}
