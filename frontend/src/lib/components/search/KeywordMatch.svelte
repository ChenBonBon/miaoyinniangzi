<script lang="ts">
 let {text,query}:{text:string;query:string}=$props()
 const index=$derived(query ? text.toLowerCase().indexOf(query.toLowerCase()) : -1)
</script>
{#if index<0}{text}{:else}{text.slice(0,index)}<mark class="bg-transparent text-highlight">{text.slice(index,index+query.length)}</mark>{text.slice(index+query.length)}{/if}
