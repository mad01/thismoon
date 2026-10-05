export type PanelPolicyRole = 'review' | 'other'

export type PanelPolicyMember = {
  id: string
  role: PanelPolicyRole
  label: string
  startedAt: number
  endedAt: number | null
  turn: string | null
  hasAgentId: boolean
}

export type PanelPolicyTurn = { id: string | null; spawns: number }

declare module 'claude-code' {
  interface PluginState {
    'panel-policy': {
      members: PanelPolicyMember[]
      turn: PanelPolicyTurn
    }
  }
}
