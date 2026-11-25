import defaults from '../internal/theme/default.json';

export type Theme = Omit<typeof defaults, 'admin_colors'> & {admin_colors: Partial<typeof defaults.admin_colors>};
export type ColorKey = keyof Theme['colors'];
export type ModuleKey = keyof Theme['modules'];
export const defaultTheme: Theme = defaults;
export const copyTheme = (theme: Theme): Theme => JSON.parse(JSON.stringify(theme));
export const adminColorKeys = Object.keys(defaults.admin_colors) as (keyof typeof defaults.admin_colors)[];
export const hexColor = /^#[0-9a-f]{6}$/i;
export function safeAsset(value: string): boolean {
  if (!value) return true;
  if (value.length > 2048 || /[\\\r\n\t"'<>]/.test(value)) return false;
  try { const u = new URL(value, 'https://theme.invalid'); return !u.username && !u.password && !u.hash && (value.startsWith('/') && !value.startsWith('//') || u.protocol === 'https:'); } catch { return false; }
}
export function parseTheme(value: unknown): Theme {
  if (!value || typeof value !== 'object') throw new Error('主题文件格式不正确');
  const t = copyTheme(value as Theme);
  if (t.brand && t.brand.bilibili_uid === undefined) t.brand.bilibili_uid = '';
  if (!t.pet) t.pet = {...defaults.pet};
  if (t.modules && !t.modules.gifts) t.modules.gifts = {...defaults.modules.gifts};
  if (t.colors && !t.colors.gifts) t.colors.gifts = t.colors.primary;
  if (t.schema_version !== 1) throw new Error('不支持此主题版本');
  for (const key of Object.keys(defaults.brand) as (keyof Theme['brand'])[]) {
    if (typeof t.brand?.[key] !== 'string' || [...t.brand[key]].length > (['logo','avatar','favicon','admin_favicon'].includes(key)?2048:300)) throw new Error('品牌信息不完整');
  }
  for (const key of ['site_name', 'streamer_name', 'tab_title', 'admin_title'] as const) if (!t.brand[key].trim() || [...t.brand[key]].length > 80) throw new Error('名称和标题请填写 1–80 个字');
  for (const key of ['logo', 'avatar', 'favicon', 'admin_favicon'] as const) if (!safeAsset(t.brand[key])) throw new Error('图片地址需要是站内路径或 HTTPS 链接');
  if (t.brand.bilibili_uid && !/^[1-9][0-9]{0,18}$/.test(t.brand.bilibili_uid)) throw new Error('B 站 UID 格式不正确');
  if (typeof t.pet.model_url !== 'string' || !safeAsset(t.pet.model_url) || typeof t.pet.label !== 'string' || [...t.pet.label].length > 80) throw new Error('桌宠地址或名称不正确');
  if (!t.colors || Object.keys(t.colors).length!==Object.keys(defaults.colors).length) throw new Error('主题颜色项不完整');
  for (const key of Object.keys(defaults.colors) as ColorKey[]) if (!hexColor.test(t.colors?.[key])) throw new Error(`颜色 ${key} 需要是 #RRGGBB 格式`);
  if (!t.admin_colors || typeof t.admin_colors !== 'object') throw new Error('后台配色格式不正确');
  for (const [key, value] of Object.entries(t.admin_colors)) if (!adminColorKeys.includes(key as typeof adminColorKeys[number]) || !hexColor.test(value)) throw new Error('后台配色格式不正确');
  for (const key of Object.keys(defaults.modules) as ModuleKey[]) {
    const m = t.modules?.[key]; if (!m || typeof m.label !== 'string' || !m.label.trim() || [...m.label].length > 20 || typeof m.icon !== 'string' || !safeAsset(m.icon)) throw new Error('模块名称或图标不正确');
  }
  for (const key of Object.keys(defaults.appearance) as (keyof Theme['appearance'])[]) if (typeof t.appearance?.[key] !== 'boolean') throw new Error('外观选项不完整');
  return copyTheme(t);
}
const rgb = (hex: string) => [1,3,5].map(i => parseInt(hex.slice(i,i+2),16)).join(',');
export function themeVariables(theme: Theme, admin = false): Record<string,string> {
  const c = {...theme.colors, ...(admin ? theme.admin_colors : {})};
  const v: Record<string,string> = {};
  for (const [key,value] of Object.entries(c)) v[`--${key}`] = value;
  v['--popover'] = c.card; v['--popover-foreground'] = c['card-foreground'];
  v['--muted'] = c.secondary; v['--input'] = c.border;
  v['--shadow-rgb'] = rgb(c.shadow); v['--primary-rgb'] = rgb(c.primary);
  v['--fabric-shadow'] = `rgba(${rgb(c.shadow)},0.08)`;
  v['--sidebar-primary'] = c.brand; v['--sidebar-primary-foreground'] = c.foreground;
  v['--sidebar-accent'] = `rgba(${rgb(c['sidebar-highlight'])},0.08)`;
  v['--sidebar-accent-foreground'] = c['sidebar-foreground'];
  v['--sidebar-border'] = v['--sidebar-accent']; v['--sidebar-ring'] = c.ring;
  if (admin) v['--fabric-stitch'] = `rgba(${rgb(c['sidebar-highlight'])},0.6)`;
  for (const key of Object.keys(theme.modules) as ModuleKey[]) v[`--module-${key}`] = c[key];
  return v;
}
export function contrast(a: string,b: string): number {
  const luminance = (s: string) => [1,3,5].map(i => parseInt(s.slice(i,i+2),16)/255).map(v => v<=.04045 ? v/12.92 : ((v+.055)/1.055)**2.4).reduce((sum,v,i)=>sum+v*[.2126,.7152,.0722][i],0);
  const x=luminance(a),y=luminance(b); return (Math.max(x,y)+.05)/(Math.min(x,y)+.05);
}
export const colorLabels: Record<ColorKey,string> = {
  background:'页面背景',foreground:'正文文字',card:'卡片与弹窗', 'card-foreground':'卡片文字',
  primary:'主色 / 按钮', 'primary-foreground':'按钮文字',secondary:'次级背景', 'secondary-foreground':'次级文字',
  'muted-foreground':'辅助文字',accent:'选中背景','accent-foreground':'选中文字',border:'边框',ring:'焦点轮廓',
  destructive:'错误 / 删除',success:'成功',warning:'提醒',brand:'前台导航背景','nav-foreground':'前台导航文字',
  sidebar:'后台侧栏背景','sidebar-foreground':'后台侧栏文字','sidebar-highlight':'侧栏高亮基色',
  'fabric-stitch':'织纹边线','fabric-grain':'织纹底色',shadow:'阴影基色',selection:'文字选中背景','selection-foreground':'文字选中颜色',viewer:'看图背景',
  'chart-1':'图表一','chart-2':'图表二','chart-3':'图表三','chart-4':'图表四','chart-5':'图表五',
  ask:'提问箱',voyage:'大航海',songs:'歌单',gallery:'画廊',gifts:'电子礼物',
};
