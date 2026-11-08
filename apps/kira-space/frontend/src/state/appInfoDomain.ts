// Type only: the bridge trusts the wire shape of AppService.Info, like every sibling method.
export interface AppInfo {
  appVersion: string;
  go: string;
  wails: string;
  kiraHome: string;
  repoUrl: string;
}
