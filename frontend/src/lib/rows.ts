import type { Process, Rule, Snapshot } from './api';

export interface Row {
  key: string;
  process: Process;
  count: number;
  download: number;
  upload: number;
  ownRule?: Rule;
  inheritedRule?: Rule;
  child: boolean;
  children: Row[];
  overrides: number;
}
export type Sort = 'name' | 'count' | 'download' | 'upload' | 'downloadLimit' | 'uploadLimit';
export const pathKey = (path: string) => path.replaceAll('/', '\\').toLowerCase();
export const identity = (p: Process) => `${p.pid}:${p.started}`;
export const displayRule = (row: Row) =>
  row.child && row.ownRule && !row.ownRule.enabled && row.inheritedRule?.enabled
    ? row.inheritedRule
    : (row.ownRule ?? row.inheritedRule);
export const displayName = (row: Row) => row.process.name.replace(/\.exe$/i, '');
export function processForRule(rule: Rule): Process {
  return {
    pid: rule.pid,
    started: rule.started,
    name: rule.name,
    path: rule.path,
    download: 0,
    upload: 0,
    downloaded: 0,
    uploaded: 0,
    ruleId: rule.id,
  };
}
export function groups(snapshot: Snapshot): Row[] {
  const applications = new Map(
    snapshot.rules.filter((r) => r.scope === 'application').map((r) => [pathKey(r.path), r]),
  );
  const overrides = new Map(
    snapshot.rules.filter((r) => r.scope === 'process').map((r) => [`${r.pid}:${r.started}`, r]),
  );
  const result = new Map<string, Row>();
  for (const p of snapshot.processes) {
    const path = pathKey(p.path);
    const key = p.path ? path : identity(p);
    let group = result.get(key);
    if (!group) {
      group = {
        key,
        process: p,
        count: 0,
        download: 0,
        upload: 0,
        ownRule: applications.get(path),
        child: false,
        children: [],
        overrides: 0,
      };
      result.set(key, group);
    }
    const candidate = overrides.get(identity(p));
    const ownRule = candidate && pathKey(candidate.path) === path ? candidate : undefined;
    group.count++;
    group.download += p.download;
    group.upload += p.upload;
    if (ownRule) group.overrides++;
    group.children.push({
      key: `process:${identity(p)}`,
      process: p,
      count: 1,
      download: p.download,
      upload: p.upload,
      ownRule,
      inheritedRule: group.ownRule,
      child: true,
      children: [],
      overrides: 0,
    });
  }
  // Retain saved applications after they exit, visible in Saved limits.
  for (const [key, rule] of applications)
    if (!result.has(key))
      result.set(key, {
        key,
        process: processForRule(rule),
        count: 0,
        download: 0,
        upload: 0,
        ownRule: rule,
        child: false,
        children: [],
        overrides: 0,
      });
  for (const row of result.values()) row.children.sort((a, b) => a.process.pid - b.process.pid);
  return [...result.values()];
}
export function visibleGroups(
  all: Row[],
  view: 'applications' | 'saved',
  filter: string,
  query: string,
  sort: Sort,
  ascending: boolean,
): Row[] {
  const text = query.toLowerCase().trim();
  const matches = (row: Row) =>
    `${row.process.name} ${row.process.path} ${row.process.pid}`.toLowerCase().includes(text);
  const filtered = all.filter(
    (r) =>
      (view === 'applications' ? r.count > 0 : !!r.ownRule || r.overrides > 0) &&
      (filter !== 'active' || r.download + r.upload > 0) &&
      (filter !== 'limited' || !!r.ownRule || r.overrides > 0) &&
      (matches(r) || r.children.some(matches)),
  );
  const value = (r: Row) =>
    sort === 'downloadLimit'
      ? (r.ownRule?.download ?? Infinity)
      : sort === 'uploadLimit'
        ? (r.ownRule?.upload ?? Infinity)
        : sort === 'name'
          ? 0
          : r[sort];
  filtered.sort(
    (a, b) =>
      (ascending ? 1 : -1) *
      ((sort === 'name' ? 0 : value(a) - value(b)) ||
        a.process.name.localeCompare(b.process.name) ||
        a.key.localeCompare(b.key)),
  );
  return filtered;
}
export function rowStatus(row: Row, snapshot: Snapshot): string {
  const r = displayRule(row);
  if (!r) return row.overrides ? 'Overrides' : '—';
  if (!r.enabled) return row.child && row.ownRule ? 'Disabled override' : 'Disabled';
  if (!snapshot.connected || snapshot.engine !== 'running') return 'Pending';
  if (snapshot.paused) return 'Paused';
  if (row.child && row.ownRule && r === row.inheritedRule) return 'Inherited';
  if (row.child && row.ownRule) return 'Override';
  return r.download === null && r.upload === null ? 'Unlimited' : 'Limited';
}
