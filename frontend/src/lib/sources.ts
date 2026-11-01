export interface SourceInfo {
  id: string
  name: string
  author: string
  version: string
  createdAt: string
  checkStatus: 'unchecked' | 'ready' | 'failed'
  checkError: string
  qualities: string[]
  active: boolean
}
export interface ImportResult { source: SourceInfo; duplicate: boolean; validationError?: string }
type SourcesAPI = {
  GetSources(): Promise<SourceInfo[]>
  ImportSource(): Promise<ImportResult | null>
  DeleteSource(id: string): Promise<boolean>
  CheckSource(id: string): Promise<SourceInfo[]>
  ActivateSource(id: string): Promise<SourceInfo[]>
  DisableSource(): Promise<SourceInfo[]>
}
function api(): SourcesAPI {
  const host = window as Window & { go?: { main?: { App?: SourcesAPI } } }
  if (!host.go?.main?.App?.GetSources) throw new Error('请在 macOS 桌面应用中管理本地音源。')
  return host.go.main.App
}
export const listSources = () => api().GetSources()
export const importSource = () => api().ImportSource()
export const deleteSource = (id: string) => api().DeleteSource(id)
export const checkSource = (id: string) => api().CheckSource(id)
export const activateSource = (id: string) => api().ActivateSource(id)
export const disableSource = () => api().DisableSource()
