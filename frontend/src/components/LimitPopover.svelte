<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { X } from 'phosphor-svelte';
  import { backend, type Snapshot } from '../lib/api';
  import { displayRule, identity, pathKey, type Row } from '../lib/rows';
  let {
    row,
    direction,
    anchor,
    snapshot,
    onclose,
    onsaved,
  }: {
    row: Row;
    direction: 'download' | 'upload';
    anchor: DOMRect;
    snapshot: Snapshot;
    onclose: () => void;
    onsaved: (s: Snapshot) => void;
  } = $props();
  const initial = untrack(() => row.ownRule ?? displayRule(row));
  const originalID = untrack(() => row.ownRule?.id);
  const initialRate = untrack(() => initial?.[direction]) ?? 1000000;
  const initialUnit = initialRate < 1000000 ? '1000' : '1000000';
  let unit = $state(initialUnit);
  let value = $state<number | undefined>(initialRate / Number(initialUnit));
  let busy = $state(false),
    error = $state('');
  let panel: HTMLDivElement;
  let input: HTMLInputElement;
  let left = $state(0),
    top = $state(0);
  function position() {
    const rect = panel.getBoundingClientRect();
    left = Math.max(8, Math.min(anchor.right - rect.width, innerWidth - rect.width - 8));
    top =
      anchor.bottom + rect.height + 8 < innerHeight
        ? anchor.bottom + 5
        : Math.max(8, anchor.top - rect.height - 5);
  }
  onMount(() => {
    position();
    input.focus();
    input.select();
    const observer = new ResizeObserver(position);
    observer.observe(panel);
    return () => observer.disconnect();
  });
  async function save(unlimited = false) {
    error = '';
    const speed = unlimited ? null : Math.round(Number(value) * Number(unit));
    if (speed !== null && (!Number.isSafeInteger(speed) || speed < 1 || speed > 125000000000)) {
      error = 'Enter a positive speed, or choose Unlimited.';
      return;
    }
    busy = true;
    try {
      const current = snapshot.rules.find(
        (r) =>
          pathKey(r.path) === pathKey(row.process.path) &&
          (row.child
            ? r.scope === 'process' && `${r.pid}:${r.started}` === identity(row.process)
            : r.scope === 'application'),
      );
      if (originalID && current?.id !== originalID)
        throw new Error('This rule changed or was removed. Close this editor and try again.');
      const inherited = row.child
        ? snapshot.rules.find(
            (r) => r.scope === 'application' && r.enabled && pathKey(r.path) === pathKey(row.process.path),
          )
        : undefined;
      const base = current ?? inherited;
      const next = {
        id: current?.id ?? '',
        name: current?.name ?? row.process.name,
        path: row.process.path,
        scope: row.child ? ('process' as const) : ('application' as const),
        pid: row.child ? row.process.pid : 0,
        started: row.child ? row.process.started : '',
        download: base?.download ?? null,
        upload: base?.upload ?? null,
        enabled: current?.enabled ?? true,
        [direction]: speed,
      };
      onsaved(await backend().SaveRule(next));
      onclose();
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
  function keyboard(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      if (!busy) onclose();
    }
    if (e.key === 'Tab') {
      const items = panel.querySelectorAll<HTMLElement>(
        'button:not(:disabled),input:not(:disabled),select:not(:disabled)',
      );
      if (e.shiftKey && document.activeElement === items[0]) {
        e.preventDefault();
        items[items.length - 1].focus();
      } else if (!e.shiftKey && document.activeElement === items[items.length - 1]) {
        e.preventDefault();
        items[0].focus();
      }
    }
  }
</script>

<svelte:window onkeydown={keyboard} onresize={position} />
<div
  class="popover-backdrop"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget && !busy) onclose();
  }}
>
  <div
    class="limit-popover"
    role="dialog"
    aria-modal="true"
    aria-labelledby="limit-title"
    bind:this={panel}
    style:left={`${left}px`}
    style:top={`${top}px`}
  >
    <form
      onsubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <div class="pop-head">
        <strong id="limit-title">{direction === 'download' ? 'Download' : 'Upload'} limit</strong><button
          type="button"
          class="icon-button"
          aria-label="Close limit editor"
          disabled={busy}
          onclick={onclose}><X size={17} /></button
        >
      </div>
      <div class="pop-body">
        <p class="pop-app">{row.process.name}{row.child ? ` · PID ${row.process.pid}` : ''}</p>
        <div class="value-field">
          <input
            aria-label="Speed limit"
            type="number"
            min="0.000001"
            step="any"
            required
            bind:value
            bind:this={input}
            disabled={busy}
          /><select aria-label="Speed unit" bind:value={unit} disabled={busy}
            ><option value="1000">KB/s</option><option value="1000000">MB/s</option><option value="125000"
              >Mbps</option
            ></select
          >
        </div>
        <div class="presets">
          {#each [250000, 1000000, 5000000] as preset}<button
              type="button"
              disabled={busy}
              onclick={() => {
                unit = preset < 1000000 ? '1000' : '1000000';
                value = preset / Number(unit);
              }}>{preset === 250000 ? '250 KB/s' : `${preset / 1000000} MB/s`}</button
            >{/each}
        </div>
        <p class="scope-note">
          {row.child
            ? 'Temporary override for this process. Removed when the process exits.'
            : 'Shared across all instances. Applies when this application opens again.'}
        </p>
        {#if row.ownRule && !row.ownRule.enabled}<p class="scope-note">
            This rule is disabled. Saving keeps it disabled; enable it in application details.
          </p>{/if}
        {#if !snapshot.connected}<p class="form-error">Connect the service to save.</p>{/if}
        {#if error}<p class="form-error" role="alert">{error}</p>{/if}
      </div>
      <div class="pop-foot">
        <button
          type="button"
          class="text-button"
          disabled={busy || !snapshot.connected}
          onclick={() => save(true)}>Set unlimited</button
        ><button class="button primary" type="submit" disabled={busy || !snapshot.connected}
          >{busy ? 'Saving…' : 'Apply'}</button
        >
      </div>
    </form>
  </div>
</div>
