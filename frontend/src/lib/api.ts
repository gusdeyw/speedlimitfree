export interface Process {
  pid: number;
  started: string;
  name: string;
  path: string;
  download: number;
  upload: number;
  downloaded: number;
  uploaded: number;
  ruleId: string;
}
export interface Rule {
  id: string;
  name: string;
  scope: 'application' | 'process';
  path: string;
  pid: number;
  started: string;
  download: number | null;
  upload: number | null;
  enabled: boolean;
}
export interface Snapshot {
  version: number;
  connected: boolean;
  engine: string;
  message: string;
  paused: boolean;
  processes: Process[];
  rules: Rule[];
  download: number;
  upload: number;
  unknownBytes: number;
  dropped: number;
  queueBytes: number;
  uptime: number;
}
export interface ServiceStatus {
  installed: boolean;
  state: string;
  startup: string;
  binary: string;
  pid: number;
  exitCode: number;
  serviceExitCode: number;
  logPath: string;
  serviceLog: string;
  setupLog: string;
}
export type ServiceAction = 'install' | 'start' | 'stop' | 'restart';
interface Backend {
  Snapshot(): Promise<Snapshot>;
  AppIcon(path: string): Promise<string>;
  SaveRule(rule: Rule): Promise<Snapshot>;
  DeleteRule(id: string): Promise<Snapshot>;
  SetPaused(paused: boolean): Promise<Snapshot>;
  SetVisible(visible: boolean): Promise<void>;
  ChooseExecutable(): Promise<string>;
  ServiceStatus(): Promise<ServiceStatus>;
  ManageService(action: ServiceAction): Promise<{ success: boolean; message: string }>;
}
declare global {
  interface Window {
    go?: { desktop: { App: Backend } };
    runtime?: {
      EventsOn(name: string, callback: (s: Snapshot) => void): () => void;
      BrowserOpenURL?(url: string): void;
    };
  }
}
export const empty: Snapshot = {
  version: 1,
  connected: false,
  engine: 'offline',
  paused: false,
  message: 'Open the desktop application to connect to the Windows service.',
  processes: [],
  rules: [],
  download: 0,
  upload: 0,
  unknownBytes: 0,
  dropped: 0,
  queueBytes: 0,
  uptime: 0,
};
export function backend(): Backend {
  if (!window.go?.desktop.App)
    throw new Error('This action is available in the Windows desktop application.');
  return window.go.desktop.App;
}
export function rate(value: number, suffix = true): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(1)} GB${suffix ? '/s' : ''}`;
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)} MB${suffix ? '/s' : ''}`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)} KB${suffix ? '/s' : ''}`;
  return `${Math.round(value)} B${suffix ? '/s' : ''}`;
}
export function limit(value: number | null | undefined) {
  return value == null ? 'Unlimited' : rate(value);
}
