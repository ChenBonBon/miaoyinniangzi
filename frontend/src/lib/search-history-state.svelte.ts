import { historyAPI, HistoryController, initialHistoryState } from '$lib/search-history'

export const historyState = $state(initialHistoryState())
export const historyController = new HistoryController(historyAPI, snapshot => Object.assign(historyState, snapshot))
