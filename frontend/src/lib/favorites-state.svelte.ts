import { toast } from 'svelte-sonner'
import { FavoritesController, initialFavoritesState, type FavoritesAPI } from './favorites-controller'

type Bridge = { GetFavorites: FavoritesAPI['load']; AddFavorite: FavoritesAPI['add']; RemoveFavorite: FavoritesAPI['remove'] }
function bridge(): Bridge {
  const api = (window as Window & { go?: { main?: { App?: Partial<Bridge> } } }).go?.main?.App
  if (!api?.GetFavorites || !api.AddFavorite || !api.RemoveFavorite) throw Error('请在桌面应用中管理收藏')
  return api as Bridge
}
export const favoritesState = $state(initialFavoritesState())
export const favoritesController = new FavoritesController({
  load: async () => bridge().GetFavorites(),
  add: async song => bridge().AddFavorite(song),
  remove: async id => bridge().RemoveFavorite(id)
}, state => Object.assign(favoritesState, state), message => toast.error(message, { id: 'favorites' }))
export function initializeFavorites() { void favoritesController.load() }
