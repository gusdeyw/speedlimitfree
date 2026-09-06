<script lang="ts">
  import { onMount, tick } from 'svelte';
  import {
    MagnifyingGlass,
    Pause,
    Play,
    Plus,
    CaretRight,
    CaretDown,
    SlidersHorizontal,
    Rows,
    X,
    AppWindow,
    WarningCircle,
  } from 'phosphor-svelte';
  import AppIcon from './components/AppIcon.svelte';
  import LimitPopover from './components/LimitPopover.svelte';
  import RuleEditor from './components/RuleEditor.svelte';
  import ServiceSettings from './components/ServiceSettings.svelte';
  import appLogo from '../../build/appicon.png';
  import { backend, empty, rate, limit, type Snapshot, type Rule, type Process } from './lib/api';
  import {
    groups,
    visibleGroups,
    displayRule,
    displayName,
    rowStatus,
    pathKey,
    type Row,
    type Sort,
  } from './lib/rows';

  let snapshot = $state<Snapshot>(empty),
    loading = $state(true);
  let view = $state<'applications' | 'saved'>('applications');
  let filter = $state('all'),
    query = $state(''),
    sort = $state<Sort>('name'),
    ascending = $state(true);
  let expanded = $state(new Set<string>()),
    selected = $state(''),
    details = $state(false);
  let settings = $state(false),
    busy = $state(false),
    error = $state(''),
    notice = $state('');
  let editor = $state<{ row: Row; direction: 'download' | 'upload'; anchor: DOMRect } | null>(null);
  let advanced = $state<{ process: Process; rule: Rule | null; scope: 'application' | 'process' } | null>(
    null,
  );
  let frozenOrder = $state<string[] | null>(null);
  let scrollTop = $state(0),
    viewportHeight = $state(500);
  let tableWidth = $state(0),
    contentWidth = $state(0);
  let search: HTMLInputElement, tableScroll: HTMLDivElement;
  let previousFocus: HTMLElement | null = null;
  const measuring = $derived(snapshot.connected && snapshot.engine === 'running');
  const all = $derived(groups(snapshot));
  const filtered = $derived(visibleGroups(all, view, filter, query, sort, ascending));
  const flatRows = $derived.by(() => {
    const text = query.trim().toLowerCase();
    const list = filtered.flatMap((row) => {
      const matchingPID = !!text && row.children.some((c) => String(c.process.pid).includes(text));
      return [row, ...(expanded.has(row.key) || matchingPID ? row.children : [])];
    });
    if (frozenOrder) {
      const rank = new Map(frozenOrder.map((key, i) => [key, i]));
      list.sort((a, b) => (rank.get(a.key) ?? Infinity) - (rank.get(b.key) ?? Infinity));
    }
    return list;
  });
  const start = $derived(
    Math.max(0, Math.min(Math.floor(scrollTop / 32) - 4, Math.max(0, flatRows.length - 1))),
  );
  const end = $derived(Math.min(flatRows.length, start + Math.ceil(viewportHeight / 32) + 8));
  const visible = $derived(flatRows.slice(start, end));
  const selectedRow = $derived(all.flatMap((r) => [r, ...r.children]).find((r) => r.key === selected));
  const maxDownload = $derived(Math.max(1, ...all.map((r) => r.download)));
  const shownProcesses = $derived(filtered.reduce((n, r) => n + r.count, 0));
  const networkCount = $derived(all.filter((r) => r.count > 0 && r.download + r.upload > 0).length);
  const limitedCount = $derived(
    all.filter((r) => (view === 'saved' || r.count > 0) && (r.ownRule || r.overrides)).length,
  );
  const columns: { key: Sort; label: string }[] = [
    { key: 'name', label: 'Application' },
    { key: 'count', label: 'Processes / PID' },
    { key: 'download', label: '↓ Download' },
    { key: 'upload', label: '↑ Upload' },
    { key: 'downloadLimit', label: 'Download limit' },
    { key: 'uploadLimit', label: 'Upload limit' },
  ];

  function receive(s: Snapshot) {
    snapshot = s;
    loading = false;
  }
  onMount(() => {
    let disposed = false;
    const receiveLive = (s: Snapshot) => {
      if (!disposed) receive(s);
    };
    if (window.go)
      void backend()
        .Snapshot()
        .then(receiveLive)
        .catch((e) => {
          if (!disposed) {
            error = String(e);
            loading = false;
          }
        });
    else loading = false;
    const off = window.runtime?.EventsOn('snapshot', receiveLive);
    const visibility = () => {
      if (!window.go) return;
      void backend()
        .SetVisible(!document.hidden)
        .catch(() => {});
      if (!document.hidden)
        void backend()
          .Snapshot()
          .then(receiveLive)
          .catch((e) => {
            if (!disposed) error = String(e);
          });
    };
    document.addEventListener('visibilitychange', visibility);
    return () => {
      disposed = true;
      off?.();
      document.removeEventListener('visibilitychange', visibility);
      if (window.go)
        void backend()
          .SetVisible(false)
          .catch(() => {});
    };
  });
  function resetScroll() {
    scrollTop = 0;
    if (tableScroll) tableScroll.scrollTop = 0;
  }
  function navigate(next: typeof view) {
    view = next;
    query = '';
    filter = 'all';
    resetScroll();
  }
  function toggleGroup(key: string) {
    const next = new Set(expanded);
    if (next.has(key)) next.delete(key);
    else {
      if (next.size >= 100) next.delete(next.values().next().value!);
      next.add(key);
    }
    expanded = next;
  }
  function select(row: Row) {
    selected = row.key;
  }
  async function edit(row: Row, direction: 'download' | 'upload', anchor: HTMLElement) {
    if (!row.process.path || !snapshot.connected) return;
    previousFocus = anchor;
    selected = row.key;
    frozenOrder = flatRows.map((r) => r.key);
    editor = { row, direction, anchor: anchor.getBoundingClientRect() };
  }
  async function closeEditor() {
    editor = null;
    advanced = null;
    frozenOrder = null;
    await tick();
    if (previousFocus?.isConnected) previousFocus.focus();
    else search?.focus();
  }
  function editAdvanced(row: Row) {
    previousFocus = document.activeElement as HTMLElement;
    advanced = {
      process: row.process,
      rule: row.ownRule ?? null,
      scope: row.child ? 'process' : 'application',
    };
    void tick().then(() => document.querySelector<HTMLButtonElement>('.drawer .icon-button')?.focus());
  }
  function saved(s: Snapshot) {
    receive(s);
    notice =
      s.engine === 'running' && !s.paused
        ? 'Rule saved.'
        : 'Rule saved. It applies when the traffic engine and limits are active.';
  }
  async function addApplication() {
    error = '';
    try {
      const path = await backend().ChooseExecutable();
      if (!path) return;
      const existing = snapshot.rules.find(
        (r) => r.scope === 'application' && pathKey(r.path) === pathKey(path),
      );
      const process: Process = {
        path,
        name: path.split(/[\\/]/).pop()!,
        pid: 0,
        started: '',
        download: 0,
        upload: 0,
        downloaded: 0,
        uploaded: 0,
        ruleId: existing?.id ?? '',
      };
      previousFocus = document.activeElement as HTMLElement;
      advanced = { process, rule: existing ?? null, scope: 'application' };
      await tick();
      document.querySelector<HTMLButtonElement>('.drawer .icon-button')?.focus();
    } catch (e) {
      error = String(e);
    }
  }
  async function pause() {
    busy = true;
    error = '';
    try {
      receive(await backend().SetPaused(!snapshot.paused));
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
  async function toggleRule(rule: Rule) {
    busy = true;
    error = '';
    try {
      receive(await backend().SaveRule({ ...rule, enabled: !rule.enabled }));
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
  async function removeRule(rule: Rule) {
    busy = true;
    error = '';
    try {
      receive(await backend().DeleteRule(rule.id));
      notice = 'Rule removed.';
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
  function keyboard(e: KeyboardEvent) {
    if (editor || advanced || settings) return;
    const inInput = ['INPUT', 'SELECT', 'TEXTAREA'].includes(
      (document.activeElement as HTMLElement)?.tagName,
    );
    if ((e.ctrlKey && e.key.toLowerCase() === 'k') || (e.key === '/' && !inInput)) {
      e.preventDefault();
      search.focus();
      search.select();
    }
    if (e.key === 'Escape' && document.activeElement === search) {
      query = '';
      resetScroll();
    }
    if (!inInput && ['ArrowDown', 'ArrowUp'].includes(e.key)) {
      e.preventDefault();
      const index = flatRows.findIndex((r) => r.key === selected);
      const next = Math.max(0, Math.min(flatRows.length - 1, index + (e.key === 'ArrowDown' ? 1 : -1)));
      if (!flatRows[next]) return;
      selected = flatRows[next].key;
      if (next * 32 < tableScroll.scrollTop) tableScroll.scrollTop = next * 32;
      else if ((next + 1) * 32 > tableScroll.scrollTop + viewportHeight)
        tableScroll.scrollTop = (next + 1) * 32 - viewportHeight;
      tableScroll.parentElement?.focus();
    }
    if (
      !inInput &&
      e.key === 'Enter' &&
      selectedRow &&
      document.activeElement === tableScroll.parentElement
    ) {
      details = true;
    }
    if (
      !inInput &&
      ['ArrowRight', 'ArrowLeft'].includes(e.key) &&
      selectedRow &&
      !selectedRow.child &&
      document.activeElement === tableScroll.parentElement
    ) {
      e.preventDefault();
      const open = expanded.has(selectedRow.key);
      if ((e.key === 'ArrowRight' && !open) || (e.key === 'ArrowLeft' && open)) toggleGroup(selectedRow.key);
    }
  }
</script>

<svelte:window onkeydown={keyboard} />
<main class="app-shell" inert={!!editor || !!advanced}>
  <header class="appbar">
    <div class="brand">
      <img src={appLogo} width="26" height="26" alt="" /><strong>speedlimit<span>free</span></strong>
    </div>
    <nav class="nav" aria-label="Views">
      <button class:active={view === 'applications'} onclick={() => navigate('applications')}
        >Applications</button
      ><button class:active={view === 'saved'} onclick={() => navigate('saved')}
        >Saved limits <span class="count">{snapshot.rules.length}</span></button
      >
    </nav>
    <span class="spacer"></span>
    <button class="service-button" onclick={() => (settings = true)} aria-label="Manage service"
      ><i class="dot" class:live={measuring}></i><span
        >{measuring ? 'Service connected' : snapshot.connected ? 'Engine inactive' : 'Service offline'}</span
      ></button
    >
    <span class="separator"></span><button
      class="icon-button"
      aria-label="Settings"
      onclick={() => (settings = true)}><SlidersHorizontal size={18} /></button
    >
  </header>
  <div class="toolbar">
    <label class="search"
      ><MagnifyingGlass size={16} /><input
        bind:this={search}
        aria-label="Search applications"
        placeholder="Search application, PID, or path"
        bind:value={query}
        oninput={resetScroll}
      /><kbd>Ctrl K</kbd></label
    >
    <div class="filters" aria-label="Filter applications">
      <button
        class:active={filter === 'all'}
        aria-pressed={filter === 'all'}
        onclick={() => {
          filter = 'all';
          resetScroll();
        }}
        >{view === 'saved' ? 'All saved' : 'All running'}
        <span class="count"
          >{view === 'saved'
            ? all.filter((r) => r.ownRule || r.overrides).length
            : all.filter((r) => r.count > 0).length}</span
        ></button
      >
      <button
        class:active={filter === 'active'}
        aria-pressed={filter === 'active'}
        disabled={!measuring}
        onclick={() => {
          filter = 'active';
          resetScroll();
        }}>Network active <span class="count">{networkCount}</span></button
      >
      <button
        class:active={filter === 'limited'}
        aria-pressed={filter === 'limited'}
        onclick={() => {
          filter = 'limited';
          resetScroll();
        }}>With limits <span class="count">{limitedCount}</span></button
      >
    </div>
    <span class="spacer"></span>
    <button
      class="text-button"
      class:active={snapshot.paused}
      onclick={pause}
      disabled={!snapshot.connected || busy}
      >{#if snapshot.paused}<Play size={16} /><span>Resume limits</span>{:else}<Pause size={16} /><span
          >Pause limits</span
        >{/if}</button
    >
    <span class="separator"></span><button
      class="text-button add"
      onclick={addApplication}
      disabled={!snapshot.connected}
      aria-label="Add application"><Plus size={16} /><span class="button-label">Add application</span></button
    >
    <button
      class="icon-button"
      aria-label="Show application details"
      aria-pressed={details}
      onclick={() => {
        if (!selected && flatRows.length) selected = flatRows[0].key;
        details = !details;
      }}><Rows size={17} /></button
    >
  </div>
  {#if error}<div class="banner error" role="alert">
      <WarningCircle size={16} /><span>{error}</span><button class="text-button" onclick={() => (error = '')}
        >Dismiss</button
      >
    </div>{/if}
  {#if !loading && !measuring}<div class="banner service-banner">
      <span
        >{snapshot.connected
          ? snapshot.message
          : 'Service offline. Start the service to measure traffic and apply limits.'}</span
      ><button class="text-button" onclick={() => (settings = true)}>Manage service</button>
    </div>{/if}
  <div
    class="process-table"
    role="grid"
    tabindex="0"
    aria-label="Application network usage"
    aria-rowcount={flatRows.length + 1}
  >
    <div
      class="table-header table-columns"
      role="row"
      style:margin-right={`${Math.max(0, tableWidth - contentWidth)}px`}
    >
      {#each columns as column}<div
          role="columnheader"
          aria-sort={sort === column.key ? (ascending ? 'ascending' : 'descending') : 'none'}
        >
          <button
            onclick={() => {
              ascending = sort === column.key ? !ascending : column.key === 'name';
              sort = column.key;
              resetScroll();
            }}
            >{column.label}<span class="sort-indicator"
              >{sort === column.key ? (ascending ? '↑' : '↓') : ''}</span
            ></button
          >
        </div>{/each}
      <div role="columnheader">Status</div>
    </div>
    <div
      class="table-scroll"
      bind:this={tableScroll}
      bind:clientHeight={viewportHeight}
      bind:offsetWidth={tableWidth}
      bind:clientWidth={contentWidth}
      onscroll={(e) => (scrollTop = e.currentTarget.scrollTop)}
      role="rowgroup"
      aria-label="Application rows"
    >
      {#if loading}<div class="empty-state">
          <p>Finding running applications…</p>
        </div>{:else if flatRows.length === 0}<div class="empty-state">
          <AppWindow size={25} />
          <h2>
            {query || filter !== 'all'
              ? 'No matching applications'
              : view === 'saved'
                ? 'No saved limits'
                : 'No applications available'}
          </h2>
          <p>
            {query || filter !== 'all'
              ? 'Try another search or change the filter.'
              : view === 'saved'
                ? 'Click a download or upload limit in Applications to create one.'
                : 'Open the Windows desktop app to discover running processes.'}
          </p>
        </div>{:else}
        <div style:height={`${start * 32}px`} role="presentation"></div>
        {#each visible as row, index (row.key)}
          {@const rule = displayRule(row)}
          {@const status = rowStatus(row, snapshot)}
          <div
            class="table-row table-columns"
            class:selected={selected === row.key}
            class:child={row.child}
            role="row"
            tabindex="-1"
            onkeydown={(e) => {
              if (e.key === 'Enter' && e.target === e.currentTarget) {
                select(row);
                details = true;
              }
            }}
            aria-rowindex={start + index + 2}
            aria-selected={selected === row.key}
            onclick={() => select(row)}
            ondblclick={(e) => {
              if (!(e.target as HTMLElement).closest('button')) {
                select(row);
                details = true;
              }
            }}
          >
            <div class="app-cell" role="gridcell" title={row.process.path || 'Executable path unavailable'}>
              {#if !row.child && row.count > 0}<button
                  class="tree"
                  aria-label={`${expanded.has(row.key) ? 'Collapse' : 'Expand'} ${displayName(row)}`}
                  aria-expanded={expanded.has(row.key)}
                  onclick={(e) => {
                    e.stopPropagation();
                    toggleGroup(row.key);
                  }}
                  >{#if expanded.has(row.key)}<CaretDown size={12} />{:else}<CaretRight
                      size={12}
                    />{/if}</button
                >{:else}<span class="tree-spacer"></span>{/if}
              <AppIcon path={row.process.path} /><span class="app-name"
                >{row.child ? row.process.name : displayName(row)}</span
              >
            </div>
            <div role="gridcell" class="number process-id">
              {row.child ? `PID ${row.process.pid}` : row.count || 'Not running'}
            </div>
            <div role="gridcell" class="number">
              <span class="speed" class:download={!!row.download && measuring}
                >{#if row.download && measuring}<i
                    class="meter"
                    style:width={`${Math.min(100, (row.download / maxDownload) * 100)}%`}
                  ></i>{/if}<span>{measuring ? rate(row.download) : '—'}</span></span
              >
            </div>
            <div role="gridcell" class="number">
              <span class="speed" class:upload={!!row.upload && measuring}
                >{measuring ? rate(row.upload) : '—'}</span
              >
            </div>
            {#each ['download', 'upload'] as direction}
              <div role="gridcell">
                <button
                  class="limit-button"
                  class:set={rule?.[direction as 'download' | 'upload'] != null}
                  class:inactive={rule && !rule.enabled}
                  aria-label={`${direction === 'download' ? 'Download' : 'Upload'} limit for ${row.process.name}${row.child ? ` PID ${row.process.pid}` : ''}`}
                  title={row.child && rule && rule === row.inheritedRule
                    ? row.ownRule
                      ? 'Application limit applies because this process override is disabled. Click to edit the disabled override.'
                      : 'Inherited application rule. Click to create a temporary process override.'
                    : 'Click to edit this limit'}
                  disabled={!row.process.path || !snapshot.connected}
                  onclick={(e) => {
                    e.stopPropagation();
                    void edit(row, direction as 'download' | 'upload', e.currentTarget);
                  }}
                  >{limit(
                    rule?.[direction as 'download' | 'upload'],
                  )}{#if row.child && rule && rule === row.inheritedRule}<span
                      class="inherited"
                      aria-label="Inherited">↳</span
                    >{/if}</button
                >
              </div>
            {/each}
            <div
              role="gridcell"
              class="row-state"
              class:limited={status === 'Limited'}
              class:attention={status === 'Paused' || status === 'Pending'}
              title={`${status}${row.overrides ? ` · ${row.overrides} process override(s)` : ''}`}
            >
              {status}{#if row.overrides && rule}<span> · {row.overrides}</span>{/if}
            </div>
          </div>
        {/each}
        <div style:height={`${(flatRows.length - end) * 32}px`} role="presentation"></div>
      {/if}
    </div>
  </div>
  {#if details}<section class="detail-strip" aria-label="Selected application details">
      {#if selectedRow}<div class="detail-name">
          <strong>{selectedRow.process.name}</strong><span
            >{selectedRow.child
              ? `Temporary process · PID ${selectedRow.process.pid}`
              : `Application · ${selectedRow.count} running process(es)`}{selectedRow.overrides
              ? ` · ${selectedRow.overrides} override(s)`
              : ''}</span
          >
        </div>
        <span class="detail-path" title={selectedRow.process.path}
          >{selectedRow.process.path || 'Executable path unavailable'}</span
        >
        <div class="detail-actions">
          {#if selectedRow.ownRule}<label class="enable-rule"
              ><input
                type="checkbox"
                aria-label={`Enable rule for ${selectedRow.process.name}`}
                checked={selectedRow.ownRule.enabled}
                disabled={busy || !snapshot.connected}
                onchange={() => toggleRule(selectedRow.ownRule!)}
              />Enabled</label
            ><button
              class="text-button"
              disabled={busy || !snapshot.connected}
              onclick={() => removeRule(selectedRow.ownRule!)}>Remove rule</button
            >{/if}<button
            class="text-button"
            aria-label={`Edit rule for ${selectedRow.process.name}`}
            disabled={!snapshot.connected || !selectedRow.process.path}
            onclick={() => editAdvanced(selectedRow)}>Edit rule</button
          >
        </div>
      {:else}<span class="scope-note">Select an application to see its path and rule controls.</span><span
          class="spacer"
        ></span>{/if}
      <button class="icon-button" aria-label="Hide application details" onclick={() => (details = false)}
        ><X size={17} /></button
      >
    </section>{/if}
  <footer class="statusbar">
    <span>{filtered.length} applications · {shownProcesses} processes</span><span
      class="total"
      title="All attributed application traffic, including packet headers"
      >↓ <strong>{measuring ? rate(snapshot.download) : '—'}</strong></span
    ><span class="total" title="All attributed application traffic, including packet headers"
      >↑ <strong>{measuring ? rate(snapshot.upload) : '—'}</strong></span
    ><span class="spacer"></span>{#if snapshot.paused}<span class="paused-status">Limits paused</span
      >{/if}<span class="status-hint">Click a limit to edit · Double-click a row for details</span>
  </footer>
</main>
{#if notice}<div class="toast" role="status">
    <span>{notice}</span><button
      class="icon-button"
      aria-label="Dismiss notification"
      onclick={() => (notice = '')}><X size={14} /></button
    >
  </div>{/if}
{#if editor}{#key editor}<LimitPopover
      row={editor.row}
      direction={editor.direction}
      anchor={editor.anchor}
      {snapshot}
      onclose={closeEditor}
      onsaved={saved}
    />{/key}{/if}
{#if advanced}{#key advanced}<RuleEditor
      process={advanced.process}
      rule={advanced.rule}
      initialScope={advanced.scope}
      connected={snapshot.connected}
      onclose={closeEditor}
      onsaved={(s) => {
        saved(s);
        if (!advanced?.process.started) navigate('saved');
      }}
    />{/key}{/if}
{#if settings}<ServiceSettings {snapshot} onclose={() => (settings = false)} onsnapshot={receive} />{/if}
