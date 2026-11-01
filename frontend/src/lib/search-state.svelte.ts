import { musicAPI } from '$lib/music'
import { initialSearchState, SearchController } from '$lib/search-controller'
import { historyController } from '$lib/search-history-state.svelte'

export const searchForm = $state({ query:'' })
export const searchState = $state(initialSearchState())
export const searchController = new SearchController(musicAPI, snapshot => Object.assign(searchState, snapshot), undefined, (query, page) => {
  if (page === 1) void historyController.record(query)
})

// Page unmounts intentionally do not dispose this controller. Requests have a
// backend deadline; users can cancel, and newer searches supersede older ones.
if (import.meta.hot) import.meta.hot.dispose(() => searchController.cancel())
