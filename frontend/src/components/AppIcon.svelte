<script lang="ts">
  import { AppWindow } from 'phosphor-svelte';
  import { loadIcon } from '../lib/icons';
  let { path }: { path: string } = $props();
  let source = $state('');
  $effect(() => {
    const controller = new AbortController();
    source = '';
    void loadIcon(path, controller.signal).then((data) => {
      if (!controller.signal.aborted) source = data;
    });
    return () => controller.abort();
  });
</script>

<span class="process-icon" aria-hidden="true">
  {#if source}<img
      src={source}
      alt=""
      width="18"
      height="18"
      onerror={() => (source = '')}
    />{:else}<AppWindow size={17} />{/if}
</span>
