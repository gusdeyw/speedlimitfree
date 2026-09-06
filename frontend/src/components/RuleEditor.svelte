<script lang="ts">
  import { untrack } from 'svelte';
  import { X, ArrowDown, ArrowUp, Trash, Check, Info } from 'phosphor-svelte';
  import { backend, type Rule, type Process, type Snapshot } from '../lib/api';
  import AppIcon from './AppIcon.svelte';
  let {
    rule,
    process,
    connected,
    onclose,
    onsaved,
    initialScope = 'application',
  }: {
    rule: Rule | null;
    process: Process;
    connected: boolean;
    onclose: () => void;
    onsaved: (s: Snapshot) => void;
    initialScope?: 'application' | 'process';
  } = $props();
  // The parent keys the editor by selection. A live snapshot must not reset edits.
  const initial = untrack(() => rule);
  const initialDownloadUnit = initial?.download && initial.download < 1_000_000 ? 'KB/s' : 'MB/s';
  const initialUploadUnit = initial?.upload && initial.upload < 1_000_000 ? 'KB/s' : 'MB/s';
  let scope = $state<'application' | 'process'>(initial?.scope ?? untrack(() => initialScope));
  let downloadUnlimited = $state(initial?.download == null);
  let uploadUnlimited = $state(initial?.upload == null);
  let downloadUnit = $state(initialDownloadUnit);
  let uploadUnit = $state(initialUploadUnit);
  let downloadValue = $state(
    initial?.download ? initial.download / (initialDownloadUnit === 'KB/s' ? 1000 : 1_000_000) : 5,
  );
  let uploadValue = $state(
    initial?.upload ? initial.upload / (initialUploadUnit === 'KB/s' ? 1000 : 1_000_000) : 1,
  );
  let enabled = $state(initial?.enabled ?? true);
  let busy = $state(false);
  let error = $state('');
  let confirmDelete = $state(false);
  let drawer: HTMLElement;
  const factors: Record<string, number> = { 'KB/s': 1000, 'MB/s': 1_000_000, Mbps: 125_000 };

  async function save(event: SubmitEvent) {
    event.preventDefault();
    error = '';
    busy = true;
    try {
      const down = downloadUnlimited ? null : Math.round(Number(downloadValue) * factors[downloadUnit]);
      const up = uploadUnlimited ? null : Math.round(Number(uploadValue) * factors[uploadUnit]);
      if ([down, up].some((v) => v !== null && (!Number.isSafeInteger(v) || v < 1 || v > 125_000_000_000)))
        throw new Error('Enter a positive speed limit, or choose Unlimited.');
      const s = await backend().SaveRule({
        id: rule?.id ?? '',
        name: rule?.name ?? process.name,
        path: process.path,
        scope,
        pid: scope === 'process' ? process.pid : 0,
        started: scope === 'process' ? process.started : '',
        download: down,
        upload: up,
        enabled,
      });
      onsaved(s);
      onclose();
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
  async function remove() {
    if (!rule) return;
    if (!confirmDelete) {
      confirmDelete = true;
      return;
    }
    busy = true;
    error = '';
    try {
      onsaved(await backend().DeleteRule(rule.id));
      onclose();
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
  function keyboard(e: KeyboardEvent) {
    if (e.key === 'Escape' && !busy) onclose();
    if (e.key !== 'Tab') return;
    const items = drawer.querySelectorAll<HTMLElement>(
      'button:not(:disabled), input:not(:disabled), select:not(:disabled)',
    );
    const first = items[0],
      last = items[items.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    }
    if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }
</script>

<svelte:window onkeydown={keyboard} />
<div
  class="drawer-backdrop"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget && !busy) onclose();
  }}
>
  <div
    class="drawer"
    role="dialog"
    aria-modal="true"
    aria-labelledby="editor-title"
    tabindex="-1"
    bind:this={drawer}
  >
    <div class="drawer-heading">
      <span class="eyebrow">BANDWIDTH RULE</span><button
        class="icon-button"
        aria-label="Close rule editor"
        onclick={onclose}
        disabled={busy}><X size={20} /></button
      >
    </div>
    <div class="editor-app">
      <AppIcon path={process.path} />
      <div>
        <h2 id="editor-title">{rule ? 'Edit limit' : 'Set a limit'}</h2>
        <p>{process.name}</p>
      </div>
    </div>
    <form onsubmit={save}>
      <label class="field-label" for="scope">Apply to</label>
      <select id="scope" bind:value={scope} disabled={!!rule}
        ><option value="application">All instances of this application</option><option
          value="process"
          disabled={!process.started}>Only this process · PID {process.pid}</option
        ></select
      >
      <p class="field-help">
        {scope === 'application'
          ? 'One shared budget across all matching processes. Saved for the next time this app opens.'
          : 'Overrides this application’s rule for this process. Expires when the process closes.'}
      </p>

      <div class="limit-field">
        <div class="limit-heading">
          <label for="download-limit"><ArrowDown size={18} weight="bold" /> Download limit</label><label
            class="checkbox-label"><input type="checkbox" bind:checked={downloadUnlimited} /> Unlimited</label
          >
        </div>
        <div class="rate-input">
          <input
            id="download-limit"
            type="number"
            min="0.000001"
            step="any"
            required={!downloadUnlimited}
            disabled={downloadUnlimited}
            bind:value={downloadValue}
          /><select aria-label="Download unit" bind:value={downloadUnit} disabled={downloadUnlimited}
            ><option>MB/s</option><option>KB/s</option><option>Mbps</option></select
          >
        </div>
      </div>
      <div class="limit-field">
        <div class="limit-heading">
          <label for="upload-limit"><ArrowUp size={18} weight="bold" /> Upload limit</label><label
            class="checkbox-label"><input type="checkbox" bind:checked={uploadUnlimited} /> Unlimited</label
          >
        </div>
        <div class="rate-input">
          <input
            id="upload-limit"
            type="number"
            min="0.000001"
            step="any"
            required={!uploadUnlimited}
            disabled={uploadUnlimited}
            bind:value={uploadValue}
          /><select aria-label="Upload unit" bind:value={uploadUnit} disabled={uploadUnlimited}
            ><option>MB/s</option><option>KB/s</option><option>Mbps</option></select
          >
        </div>
      </div>

      <div class="rule-enabled">
        <div>
          <strong>Enable this rule</strong>
          <p>Start applying the limit when available.</p>
        </div>
        <label class="switch"
          ><input aria-label="Enable this rule" type="checkbox" bind:checked={enabled} /><span></span></label
        >
      </div>
      <div class="info-note">
        <Info size={18} />
        <p>
          Download limits control delivery to the app. Incoming traffic may still use your connection before
          it is shaped.
        </p>
      </div>
      <div class="executable">
        <span class="field-label">Executable</span><code>{process.path || 'Path unavailable'}</code>
      </div>
      {#if error}<p class="form-error" role="alert">{error}</p>{/if}
      {#if !connected}<p class="form-error">Connect the background service to save rules.</p>{/if}
      <div class="editor-actions">
        <button type="button" class="button secondary" onclick={onclose} disabled={busy}>Cancel</button
        ><button class="button primary" type="submit" disabled={busy || !connected || !process.path}
          ><Check size={17} />{busy ? 'Saving…' : 'Save rule'}</button
        >
      </div>
      {#if rule}<button type="button" class="delete-rule" onclick={remove} disabled={busy}
          ><Trash size={16} />{confirmDelete ? 'Confirm delete rule' : 'Delete this rule'}</button
        >{/if}
    </form>
  </div>
</div>
