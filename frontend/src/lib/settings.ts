export interface Settings {
  downloadDirectory: string
  downloadQuality: string
  playbackQuality: string
  volume: number
}

type SettingsAPI = {
  GetSettings(): Promise<Settings>
  SaveSettings(settings: Settings): Promise<void>
  ChooseDownloadDirectory(initialDirectory: string): Promise<string>
}

function api(): SettingsAPI {
  const host = window as Window & { go?: { main?: { App?: SettingsAPI } } }
  const bridge = host.go?.main?.App
  if (!bridge) throw new Error('请在 macOS 桌面应用中修改设置，浏览器预览不支持本地保存。')
  return bridge
}

export const loadSettings = () => api().GetSettings()
export const saveSettings = (settings: Settings) => api().SaveSettings(settings)
export const chooseDownloadDirectory = (initialDirectory: string) => api().ChooseDownloadDirectory(initialDirectory)
export const errorMessage = (error: unknown) => error instanceof Error ? error.message : String(error)
