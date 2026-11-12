import {previewSongs,samplePreviewSongs} from './for-you-preview'
import {navigate} from './navigation.svelte'

// Only local prototype state; never serialized into playback, favorites or DB.
export const forYouPreview=$state({songs:samplePreviewSongs(),batch:1,albumID:1})
export function replacePreviewBatch(){forYouPreview.songs=samplePreviewSongs();forYouPreview.batch++}
export function openPreviewAlbum(id:number){
 if(!previewSongs.some(song=>song.id===id))return
 navigate('专辑预览',()=>forYouPreview.albumID=id)
}
