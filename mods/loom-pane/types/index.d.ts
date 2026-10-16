export type LoomPaneRole = 'owner' | 'weaver' | 'none'

/** One member on the status line; a field no source recorded is `-`, `state` is a session's CLI state. */
export type LoomPaneMember = {
  name: string
  role: string
  worktree: string
  model: string
  effort: string
  state: string | null
}

declare module 'claude-code' {
  interface PluginState {
    'loom-pane': {
      role: LoomPaneRole
      canonical: string | null
      ownWorktree: string | null
      worktreeRoot: string | null
      worktrees: string[]
      repo: string | null
      weaveSeen: boolean
      isInteractive: boolean
      members: LoomPaneMember[]
    }
  }
}
