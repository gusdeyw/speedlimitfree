<script lang="ts">
  import { onMount } from 'svelte';
  import { X, ArrowClockwise } from 'phosphor-svelte';
  import { backend, rate, type Snapshot, type ServiceStatus, type ServiceAction } from '../lib/api';
  import appLogo from '../../../build/appicon.png';
  import { version } from '../../package.json';
  let {
    snapshot,
    onclose,
    onsnapshot,
  }: { snapshot: Snapshot; onclose: () => void; onsnapshot: (s: Snapshot) => void } = $props();
  let service = $state<ServiceStatus | null>(null),
    error = $state(''),
    notice = $state('');
  let busy = $state<ServiceAction | ''>(''),
    refreshing = $state(false);
  let dialog: HTMLDialogElement;
  const pending = $derived(
    !!service && ['starting', 'stopping', 'pausing', 'resuming'].includes(service.state),
  );
  async function refresh() {
    refreshing = true;
    try {
      service = await backend().ServiceStatus();
    } catch (e) {
      error = String(e);
      service = null;
    } finally {
      refreshing = false;
    }
  }
  async function manage(action: ServiceAction) {
    if (busy) return;
    busy = action;
    error = '';
    notice = '';
    try {
      const result = await backend().ManageService(action);
      if (!result.success) throw new Error(result.message);
      notice = result.message;
    } catch (e) {
      error = String(e);
    } finally {
      await refresh();
      try {
        onsnapshot(await backend().Snapshot());
      } catch (e) {
        error ||= String(e);
      }
      busy = '';
    }
  }
  onMount(() => {
    dialog.showModal();
    void refresh();
  });
</script>

<dialog
  class="settings-dialog"
  bind:this={dialog}
  oncancel={(e) => {
    e.preventDefault();
    if (!busy) onclose();
  }}
  aria-labelledby="settings-title"
>
  <div class="pop-head">
    <strong id="settings-title">Service and settings</strong><button
      class="icon-button"
      aria-label="Close settings"
      disabled={!!busy}
      onclick={onclose}><X size={18} /></button
    >
  </div>
  <div class="settings-body">
    <div class="settings-row">
      <div>
        <strong>Windows service</strong>
        <p>
          {service?.installed
            ? `Startup: ${service.startup}${service.pid ? ` · PID ${service.pid}` : ''}`
            : 'Install once, then use Start, Stop, or Restart.'}
        </p>
      </div>
      <div class="service-actions">
        <span class="service-state">{service?.state ?? (refreshing ? 'checking' : 'unavailable')}</span
        ><button
          class="icon-button"
          aria-label="Refresh service status"
          disabled={refreshing || !!busy}
          onclick={refresh}><ArrowClockwise size={17} /></button
        >
      </div>
    </div>
    <div class="service-actions controls">
      <button
        class="button secondary"
        disabled={!service?.installed || service.state !== 'stopped' || !!busy}
        onclick={() => manage('start')}>Start</button
      ><button
        class="button secondary"
        disabled={!service?.installed || !['running', 'paused'].includes(service.state) || !!busy}
        onclick={() => manage('stop')}>Stop</button
      ><button
        class="button secondary"
        disabled={!service?.installed || !['running', 'paused'].includes(service.state) || !!busy}
        onclick={() => manage('restart')}>Restart</button
      ><span class="spacer"></span><button
        class="button secondary"
        disabled={!!busy || pending || refreshing}
        onclick={() => manage('install')}>Install / repair</button
      >
    </div>
    <p class="scope-note">
      Stopping disables traffic limits and keeps saved rules. Windows asks for administrator access.
    </p>
    {#if busy}<p class="operation" role="status">
        {{ install: 'Installing / repairing', start: 'Starting', stop: 'Stopping', restart: 'Restarting' }[
          busy
        ]} the service… Complete the Windows administrator prompt. Waiting for the result.
      </p>{/if}
    {#if notice}<p class="operation" role="status">{notice}</p>{/if}
    {#if error}<p class="form-error" role="alert">{error}</p>{/if}
    <div class="settings-row">
      <div>
        <strong>Traffic engine</strong>
        <p>{snapshot.message}</p>
      </div>
      <span class="service-state">{snapshot.engine}</span>
    </div>
    {#if service && (service.exitCode || service.serviceExitCode)}<p class="form-error">
        Windows exit code: {service.exitCode}. Service exit code: {service.serviceExitCode}.
      </p>{/if}
    <details class="service-logs">
      <summary>Service and setup logs</summary>
      <p>{service?.logPath ?? 'Refresh to load logs.'}</p>
      <strong>Latest setup actions</strong>
      <pre>{service?.setupLog || 'No setup log yet.'}</pre>
      <strong>Service log</strong>
      <pre>{service?.serviceLog || 'No service log entries yet.'}</pre>
    </details>
    <details class="service-logs">
      <summary>Traffic diagnostics</summary>
      <dl>
        <dt>Queued traffic</dt>
        <dd>{rate(snapshot.queueBytes, false)}</dd>
        <dt>Dropped packets</dt>
        <dd>{snapshot.dropped.toLocaleString()}</dd>
        <dt>Unattributed traffic</dt>
        <dd>{rate(snapshot.unknownBytes, false)}</dd>
        <dt>Service uptime</dt>
        <dd>{Math.floor(snapshot.uptime / 60)} min</dd>
      </dl>
      <p>
        Uncertain process ownership passes through. Rates count IP packets, including headers. Download limits
        control delivery after traffic reaches your PC.
      </p>
    </details>
    <div class="settings-row">
      <div>
        <strong>Appearance</strong>
        <p>Follows the Windows app theme automatically.</p>
      </div>
      <span class="service-state">System</span>
    </div>
    <div class="settings-row">
      <div>
        <strong>Close to tray</strong>
        <p>Close hides this window. Quit from the tray releases the desktop; the service keeps running.</p>
      </div>
    </div>
    <div class="app-about">
      <img src={appLogo} width="36" height="36" alt="" />
      <div>
        <p>SpeedLimitFree <span class="version">{version}</span></p>
        <p class="creator">
          Created by <a
            href="https://github.com/gusdeyw"
            target="_blank"
            rel="noopener noreferrer"
            onclick={(event) => {
              if (window.runtime?.BrowserOpenURL) {
                event.preventDefault();
                window.runtime.BrowserOpenURL('https://github.com/gusdeyw');
              }
            }}>gusdeyw</a
          >
        </p>
      </div>
    </div>
  </div>
</dialog>
