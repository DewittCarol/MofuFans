'use client';

import { useEffect, useRef, useState, type CSSProperties } from 'react';
import { Palette, Save, Upload, Download, RotateCcw, Trash2, MailOpen, Sailboat, Music2, Images, Gift } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Select, SelectTrigger, SelectContent, SelectItem, SelectValue } from '@/components/ui/select';
import { useSiteTheme } from '@/lib/site-theme';
import { FooterSettings } from '@/components/footer-settings';
import { BackgroundSettings } from '@/components/background-settings';
import { EmojiSettings } from '@/components/emoji-settings';
import { getSettings, updateSettings, type CursorTheme } from '@/lib/api';
import { adminColorKeys, colorLabels, contrast, copyTheme, defaultTheme, hexColor, parseTheme, safeAsset, themeVariables, type ColorKey, type ModuleKey, type Theme } from '../../../../../shared/theme';

type Preset={id:string;name:string;builtin:boolean;theme:Theme};
async function request<T>(path:string,init?:RequestInit):Promise<T>{
  const res=await fetch(path,{credentials:'include',cache:'no-store',...init});
  if(!res.ok)throw new Error(`请求失败（${res.status}）`);
  const body=await res.json();if(body.code!==200)throw new Error(body.message||'操作失败');return body.data;
}
const json=(value:unknown)=>({headers:{'Content-Type':'application/json'},body:JSON.stringify(value)});
const moduleKeys=Object.keys(defaultTheme.modules) as ModuleKey[];
const moduleIcons={ask:MailOpen,voyage:Sailboat,songs:Music2,gallery:Images,gifts:Gift};
const mainColors:ColorKey[]=['background','foreground','card','card-foreground','primary','primary-foreground','secondary','secondary-foreground','muted-foreground','accent','accent-foreground','border','ring'];
const detailColors:ColorKey[]=['destructive','success','warning','selection','selection-foreground','viewer','fabric-stitch','fabric-grain','shadow','chart-1','chart-2','chart-3','chart-4','chart-5'];

function ColorField({name,value,onChange}:{name:string;value:string;onChange:(v:string)=>void}){
  const id=`color-${name}`;
  return <div className="space-y-2"><Label htmlFor={id}>{name}</Label><div className="flex items-center gap-2">
    <input aria-label={`${name}取色器`} type="color" value={hexColor.test(value)?value:'#000000'} onChange={e=>onChange(e.target.value)} className="h-10 w-11 shrink-0 cursor-pointer rounded-md border border-border bg-card p-1"/>
    <Input id={id} value={value} onChange={e=>onChange(e.target.value)} maxLength={7} spellCheck={false} aria-invalid={!hexColor.test(value)} className="font-mono text-sm"/>
  </div></div>;
}
function ImageField({name,value,onChange,onError}:{name:string;value:string;onChange:(v:string)=>void;onError:(s:string)=>void}){
  const [busy,setBusy]=useState(false);const file=useRef<HTMLInputElement>(null);const id=`asset-${name}`;
  const upload=async(image?:File)=>{
    if(!image)return;if(image.size>12*1024*1024){onError('图片不能超过 12 MB');return;}
    setBusy(true);try{const form=new FormData();form.append('file',image);const result=await request<{url:string}>('/api/theme-assets',{method:'POST',body:form});onChange(result.url);}catch(e){onError((e as Error).message);}finally{setBusy(false);if(file.current)file.current.value='';}
  };
  return <div className="space-y-2"><Label htmlFor={id}>{name}</Label><div className="flex items-start gap-3">
    <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg border border-border bg-secondary">{value&&safeAsset(value)?<img src={value} alt={`${name}预览`} className="h-10 w-10 object-contain"/>:<Images size={20} className="text-muted-foreground"/>}</div>
    <div className="min-w-0 flex-1 space-y-2"><Input id={id} value={value} onChange={e=>onChange(e.target.value)} placeholder="上传图片，或填写 HTTPS 图片链接" maxLength={2048}/><div className="flex gap-2"><Button size="sm" variant="outline" disabled={busy} onClick={()=>file.current?.click()}><Upload size={14}/>{busy?'上传中…':'上传图片'}</Button>{value&&<Button size="sm" variant="ghost" onClick={()=>onChange('')}>移除</Button>}</div></div>
    <input ref={file} type="file" accept="image/png,image/jpeg,image/webp" hidden onChange={e=>void upload(e.target.files?.[0])}/>
  </div></div>;
}
function Preview({theme,admin}:{theme:Theme;admin:boolean}){
  return <div style={themeVariables(theme,admin) as CSSProperties} className="overflow-hidden rounded-xl border border-border bg-background text-foreground shadow-sm">
    <div style={{background:admin?'var(--sidebar)':'var(--brand)',color:admin?'var(--sidebar-foreground)':'var(--nav-foreground)'}} className="flex min-h-12 items-center gap-2 px-4 py-3">
      {theme.brand.logo&&<img src={theme.brand.logo} alt="" className="h-7 w-7 object-contain"/>}<span className="truncate text-sm font-medium">{admin?theme.brand.admin_title:theme.brand.site_name}</span>
    </div>
    <div className="space-y-5 p-5"><div className="text-center">{theme.brand.avatar&&<img src={theme.brand.avatar} alt="" className="mx-auto mb-2 h-14 w-14 object-contain"/>}<p className="font-semibold">{theme.brand.streamer_name}</p></div>
      <div className="grid grid-cols-2 gap-2">{moduleKeys.map(key=>{const Icon=moduleIcons[key],m=theme.modules[key];return <div key={key} className="flex items-center gap-2 rounded-lg border border-border bg-card p-3 text-sm text-card-foreground">{m.icon?<img src={m.icon} alt="" className="h-5 w-5 object-contain"/>:<Icon size={20} style={{color:theme.appearance.module_colors?theme.colors[key]:theme.colors.primary}}/>}{m.label}</div>;})}</div>
      <div className="space-y-3 rounded-lg border border-border bg-card p-4 text-card-foreground"><p className="font-medium">卡片标题</p><p className="text-sm text-muted-foreground">这里是辅助文字和表单样式。</p><div className="rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground">输入内容</div><div className="flex gap-2 text-sm"><span className="rounded-md bg-primary px-3 py-2 text-primary-foreground">主要按钮</span><span className="rounded-md bg-secondary px-3 py-2 text-secondary-foreground">次要按钮</span></div><div className="flex gap-3 text-xs"><span className="text-success">成功</span><span className="text-warning">提醒</span><span className="text-destructive">错误</span></div></div>
    </div>
  </div>;
}

export default function ThemePage(){
  const {theme,revision,ready,refresh}=useSiteTheme();
  const [draft,setDraft]=useState<Theme>(()=>copyTheme(defaultTheme));
  const [saved,setSaved]=useState<Theme>(()=>copyTheme(defaultTheme));
  const [baseRevision,setBaseRevision]=useState(0);
  const initialized=useRef(false);const importFile=useRef<HTMLInputElement>(null);
  const [presets,setPresets]=useState<Preset[]>([]),[selected,setSelected]=useState('default'),[presetName,setPresetName]=useState('');
  const [busy,setBusy]=useState(false),[message,setMessage]=useState(''),[error,setError]=useState(''),[deletePending,setDeletePending]=useState(false),[previewAdmin,setPreviewAdmin]=useState(false);
  const [area,setArea]=useState('site');
  type Behavior={cursor_theme:string;pet_default_expanded:boolean;allow_repeated_reactions:boolean};
  const [behavior,setBehavior]=useState<Behavior|null>(null),[savedBehavior,setSavedBehavior]=useState<Behavior|null>(null),[cursorThemes,setCursorThemes]=useState<CursorTheme[]>([]);
  useEffect(()=>{if(ready&&!initialized.current){initialized.current=true;setDraft(copyTheme(theme));setSaved(copyTheme(theme));setBaseRevision(revision);}},[theme,ready,revision]);
  useEffect(()=>{void request<Preset[]>('/api/theme/presets').then(setPresets).catch(e=>setError(e.message));},[]);
  useEffect(()=>{void getSettings().then(result=>{if(result.code!==200)throw new Error(result.message||'外观设置加载失败');const value={cursor_theme:result.data.cursor_theme||'system',pet_default_expanded:!!result.data.pet_default_expanded,allow_repeated_reactions:!!result.data.allow_repeated_reactions};setBehavior(value);setSavedBehavior(value);setCursorThemes(result.data.cursor_themes||[]);}).catch(e=>setError(e.message));},[]);
  const themeDirty=JSON.stringify(draft)!==JSON.stringify(saved);
  const behaviorDirty=JSON.stringify(behavior)!==JSON.stringify(savedBehavior);
  const dirty=themeDirty||behaviorDirty;
  useEffect(()=>{if(!dirty)return;const guard=(e:BeforeUnloadEvent)=>{e.preventDefault();};window.addEventListener('beforeunload',guard);return()=>window.removeEventListener('beforeunload',guard);},[dirty]);
  let valid:Theme|null=null;let validation='';try{valid=parseTheme(draft);}catch(e){validation=(e as Error).message;}
  const preview=valid||saved;
  const previewColors={...preview.colors,...(previewAdmin?preview.admin_colors:{})};
  const lowContrast=[['正文',previewColors.foreground,previewColors.background],['按钮',previewColors['primary-foreground'],previewColors.primary],['卡片',previewColors['card-foreground'],previewColors.card],['辅助文字',previewColors['muted-foreground'],previewColors.background]].filter(([,a,b])=>contrast(a,b)<4.5).map(([label])=>label);
  const brand=<K extends keyof Theme['brand']>(key:K,value:string)=>setDraft(t=>({...t,brand:{...t.brand,[key]:value}}));
  const color=(key:ColorKey,value:string)=>setDraft(t=>({...t,colors:{...t.colors,[key]:value}}));
  const act=async(action:()=>Promise<void>)=>{setBusy(true);setMessage('');setError('');try{await action();}catch(e){setError((e as Error).message);}finally{setBusy(false);}};
  const save=()=>act(async()=>{
    if(themeDirty){const validated=parseTheme(draft);const result=await request<{theme:Theme;revision:number}>('/api/theme',{method:'PUT',...json({theme:validated,revision:baseRevision})});setSaved(copyTheme(result.theme));setDraft(copyTheme(result.theme));setBaseRevision(result.revision);await refresh();}
    if(behaviorDirty&&behavior&&savedBehavior){const patch:Partial<Behavior>={};for(const key of Object.keys(behavior) as (keyof Behavior)[]){if(behavior[key]!==savedBehavior[key])Object.assign(patch,{[key]:behavior[key]});}const result=await updateSettings(patch);if(result.code!==200)throw new Error(result.message||'外观设置保存失败，请重试');setSavedBehavior({...behavior});}
    try{localStorage.setItem('site-theme-updated',String(Date.now()));}catch{/* Storage may be disabled. */}setMessage('设置已保存。');
  });
  const exportTheme=()=>{try{const data=parseTheme(draft);const url=URL.createObjectURL(new Blob([JSON.stringify(data,null,2)],{type:'application/json'}));const a=document.createElement('a');a.href=url;a.download='site-theme.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}catch(e){setError((e as Error).message);}};
  const importTheme=async(file?:File)=>{if(!file)return;try{if(file.size>64*1024)throw new Error('主题文件不能超过 64 KB');setDraft(parseTheme(JSON.parse(await file.text())));setMessage('主题已载入草稿，保存后生效。');setError('');}catch(e){setError((e as Error).message);}finally{if(importFile.current)importFile.current.value='';}};
  return <div className="space-y-6">
    <div className="flex flex-wrap items-center justify-between gap-3"><h1 className="flex items-center gap-2 text-2xl font-semibold"><Palette size={25}/>主题管理</h1><span className="text-sm text-muted-foreground">{dirty?'有未保存的修改':'当前主题'}</span></div>
    {!ready&&<p role="status" className="text-sm text-muted-foreground">正在读取主题… <Button size="sm" variant="outline" onClick={()=>void refresh()}>重新读取</Button></p>}
    {error&&<p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{error}</p>}
    {message&&<p role="status" className="text-sm text-success">{message}</p>}
    {baseRevision>0&&revision!==baseRevision&&<div role="alert" className="rounded-lg border border-border bg-secondary p-3 text-sm">其他页面已更新主题，请先导出保留你的草稿，再重新载入。<Button className="ml-2" size="sm" variant="outline" onClick={()=>{setDraft(copyTheme(theme));setSaved(copyTheme(theme));setBaseRevision(revision);}}>载入最新主题</Button></div>}
    <Tabs value={area} onValueChange={setArea}>
    <TabsList className="mb-6 flex h-auto flex-wrap justify-start gap-1" aria-label="主题设置分区"><TabsTrigger value="site">站点外观</TabsTrigger><TabsTrigger value="background">背景图</TabsTrigger><TabsTrigger value="effects">鼠标与桌宠</TabsTrigger><TabsTrigger value="emojis">表情管理</TabsTrigger><TabsTrigger value="footer">页脚备案</TabsTrigger></TabsList>
    <TabsContent value="site">
    <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
      <div className="min-w-0 rounded-xl border border-border bg-card p-4 sm:p-6"><Tabs defaultValue="brand">
        <TabsList className="mb-6 flex h-auto flex-wrap justify-start gap-1"><TabsTrigger value="brand">品牌信息</TabsTrigger><TabsTrigger value="colors">基础配色</TabsTrigger><TabsTrigger value="modules">模块配色</TabsTrigger><TabsTrigger value="presets">主题预设</TabsTrigger></TabsList>
        <TabsContent value="brand" className="space-y-7">
          <div className="grid gap-5 sm:grid-cols-2">{([['site_name','网站名称'],['streamer_name','主播名称'],['tab_title','前台页签标题'],['admin_title','后台标题']] as const).map(([key,label])=><div key={key} className="space-y-2"><Label htmlFor={key}>{label}</Label><Input id={key} maxLength={80} value={draft.brand[key]} onChange={e=>brand(key,e.target.value)}/></div>)}</div>
          <div className="space-y-2"><Label htmlFor="description">网站描述</Label><Input id="description" maxLength={300} value={draft.brand.description} onChange={e=>brand('description',e.target.value)}/></div>
          <div className="space-y-2"><Label htmlFor="bilibili_uid">主播 B 站 UID（可留空）</Label><Input id="bilibili_uid" inputMode="numeric" maxLength={19} value={draft.brand.bilibili_uid} onChange={e=>brand('bilibili_uid',e.target.value)}/><p className="text-xs text-muted-foreground">填写后，首页显示对应的 B 站主页入口。</p></div>
          <div className="grid gap-6 sm:grid-cols-2">{([['logo','导航图标'],['avatar','首页头像'],['favicon','前台页签图标'],['admin_favicon','后台页签图标']] as const).map(([key,label])=><ImageField key={key} name={label} value={draft.brand[key]} onChange={v=>brand(key,v)} onError={setError}/>)}</div>
          <p className="text-xs text-muted-foreground">图片支持 PNG、JPG、WebP，最大 12 MB。页签图标建议使用正方形 PNG。</p>
          <div className="grid gap-4 sm:grid-cols-2">{([['texture','显示织物纹理'],['pixel_logo','图标使用像素显示']] as const).map(([key,label])=><label key={key} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={draft.appearance[key]} onChange={e=>setDraft(t=>({...t,appearance:{...t.appearance,[key]:e.target.checked}}))} className="h-4 w-4 accent-primary"/>{label}</label>)}</div>
        </TabsContent>
        <TabsContent value="colors" className="space-y-7">
          <div className="grid gap-5 sm:grid-cols-2">{mainColors.map(key=><ColorField key={key} name={colorLabels[key]} value={draft.colors[key]} onChange={v=>color(key,v)}/>)}</div>
          <details className="border-t border-border pt-4"><summary className="cursor-pointer font-medium">状态、织纹与图表</summary><div className="mt-5 grid gap-5 sm:grid-cols-2">{detailColors.map(key=><ColorField key={key} name={colorLabels[key]} value={draft.colors[key]} onChange={v=>color(key,v)}/>)}</div></details>
          <details className="border-t border-border pt-4"><summary className="cursor-pointer font-medium">后台独立配色</summary><p className="my-3 text-sm text-muted-foreground">关闭独立配色后，后台跟随基础配色。</p><label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-primary" checked={Object.keys(draft.admin_colors).length>0} onChange={e=>setDraft(t=>({...t,admin_colors:e.target.checked?Object.fromEntries(adminColorKeys.map(k=>[k,t.colors[k]])): {}}))}/>启用后台独立配色</label>{Object.keys(draft.admin_colors).length>0&&<div className="mt-5 grid gap-5 sm:grid-cols-2">{adminColorKeys.map(key=><ColorField key={key} name={`后台${colorLabels[key]}`} value={draft.admin_colors[key]||draft.colors[key]} onChange={v=>setDraft(t=>({...t,admin_colors:{...t.admin_colors,[key]:v}}))}/>)}</div>}</details>
        </TabsContent>
        <TabsContent value="modules" className="space-y-7">
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-primary" checked={draft.appearance.module_colors} onChange={e=>setDraft(t=>({...t,appearance:{...t.appearance,module_colors:e.target.checked}}))}/>使用各模块独立主题色（关闭后跟随主色）</label>
          <div className="grid gap-5 sm:grid-cols-2">{(['brand','nav-foreground','sidebar','sidebar-foreground','sidebar-highlight'] as ColorKey[]).map(key=><ColorField key={key} name={colorLabels[key]} value={draft.colors[key]} onChange={v=>color(key,v)}/>)}</div>
          {moduleKeys.map(key=><section key={key} className="space-y-4 border-t border-border pt-5"><h2 className="font-medium">{defaultTheme.modules[key].label}</h2><div className="grid gap-5 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor={`module-${key}`}>入口名称</Label><Input id={`module-${key}`} value={draft.modules[key].label} maxLength={20} onChange={e=>setDraft(t=>({...t,modules:{...t.modules,[key]:{...t.modules[key],label:e.target.value}}}))}/></div>{draft.appearance.module_colors&&<ColorField name={`${colorLabels[key]}主题色`} value={draft.colors[key]} onChange={v=>color(key,v)}/>}</div><ImageField name={`${defaultTheme.modules[key].label}入口图标`} value={draft.modules[key].icon} onChange={v=>setDraft(t=>({...t,modules:{...t.modules,[key]:{...t.modules[key],icon:v}}}))} onError={setError}/></section>)}
        </TabsContent>
        <TabsContent value="presets" className="space-y-6">
          <div className="space-y-3"><Label>选择预设</Label><Select value={selected} onValueChange={v=>{setSelected(v);setDeletePending(false);}}><SelectTrigger aria-label="选择主题预设"><SelectValue/></SelectTrigger><SelectContent>{presets.map(p=><SelectItem key={p.id} value={p.id}>{p.name}{p.builtin?' · 内置':''}</SelectItem>)}</SelectContent></Select><div className="flex flex-wrap gap-2"><Button variant="outline" disabled={busy||!presets.length} onClick={()=>{const p=presets.find(p=>p.id===selected);if(p){setDraft(copyTheme(p.theme));setMessage('预设已载入草稿，保存后生效。');setError('');}}}><RotateCcw size={15}/>载入预设</Button>{!!presets.find(p=>p.id===selected&&!p.builtin)&&<Button variant="ghost" disabled={busy} onClick={()=>setDeletePending(!deletePending)}><Trash2 size={15}/>删除预设</Button>}</div>{deletePending&&<div className="rounded-lg border border-border p-3 text-sm">删除“{presets.find(p=>p.id===selected)?.name}”？当前使用的主题不受影响。<div className="mt-2 flex gap-2"><Button size="sm" variant="destructive" disabled={busy} onClick={()=>void act(async()=>{await request(`/api/theme/presets/${selected}`,{method:'DELETE'});setPresets(p=>p.filter(x=>x.id!==selected));setSelected('default');setDeletePending(false);})}>确认删除</Button><Button size="sm" variant="outline" onClick={()=>setDeletePending(false)}>取消</Button></div></div>}</div>
          <div className="space-y-3 border-t border-border pt-5"><Label htmlFor="preset-name">将草稿另存为预设</Label><Input id="preset-name" value={presetName} maxLength={80} onChange={e=>setPresetName(e.target.value)} placeholder="预设名称"/><Button variant="outline" disabled={busy||!presetName.trim()||!!validation} onClick={()=>void act(async()=>{const p=await request<Preset>('/api/theme/presets',{method:'POST',...json({name:presetName,theme:parseTheme(draft)})});setPresets(list=>[...list,p]);setSelected(p.id);setPresetName('');setMessage('预设已保存，当前网站主题未改变。');})}>保存为新预设</Button></div>
          <div className="space-y-3 border-t border-border pt-5"><div className="flex flex-wrap gap-2"><Button variant="outline" onClick={exportTheme}><Download size={15}/>导出主题</Button><Button variant="outline" onClick={()=>importFile.current?.click()}><Upload size={15}/>导入主题</Button><input ref={importFile} type="file" accept="application/json,.json" hidden onChange={e=>void importTheme(e.target.files?.[0])}/></div><p className="text-xs text-muted-foreground">主题文件包含配置和图片地址；迁移到其他网站时，需要一并保留图片资源。</p></div>
        </TabsContent>
      </Tabs></div>
      <aside className="space-y-3 xl:sticky xl:top-0"><div className="flex items-center justify-between"><h2 className="text-sm font-medium">草稿预览</h2><Button variant="outline" size="sm" onClick={()=>setPreviewAdmin(v=>!v)}>{previewAdmin?'查看前台':'查看后台'}</Button></div><Preview theme={preview} admin={previewAdmin}/>{lowContrast.length>0&&<p className="text-xs text-warning">{lowContrast.join('、')}的文字对比度偏低，建议调整颜色。</p>}<p className="text-xs text-muted-foreground">保存后应用到前台和后台。</p></aside>
    </div>
    </TabsContent>
    <TabsContent value="background"><BackgroundSettings /></TabsContent>
    <TabsContent value="effects" className="max-w-3xl space-y-6">
      <section className="space-y-5 rounded-xl border border-border bg-card p-5 sm:p-6" aria-labelledby="cursor-heading"><h2 id="cursor-heading" className="text-lg font-semibold">鼠标指针</h2>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-primary" checked={draft.appearance.cursor} onChange={e=>setDraft(t=>({...t,appearance:{...t.appearance,cursor:e.target.checked}}))}/>启用自定义鼠标</label>
        <div className="space-y-2"><Label htmlFor="cursor-theme">鼠标套装</Label><Select value={behavior?.cursor_theme||'system'} onValueChange={value=>setBehavior(current=>current?{...current,cursor_theme:value}:current)} disabled={!behavior||busy}><SelectTrigger id="cursor-theme"><SelectValue /></SelectTrigger><SelectContent>{cursorThemes.map(item=><SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select><p className="text-sm text-muted-foreground">选择已安装的套装，手机触屏不受影响。通用版仅附系统指针，自定义套装需自行制作并安装。</p></div>
        <div className="space-y-2"><Label htmlFor="cursor_message">鼠标设置说明（可留空）</Label><Input id="cursor_message" maxLength={300} value={draft.brand.cursor_message} onChange={e=>brand('cursor_message',e.target.value)}/></div>
      </section>
      <section className="space-y-5 rounded-xl border border-border bg-card p-5 sm:p-6" aria-labelledby="pet-heading"><h2 id="pet-heading" className="text-lg font-semibold">桌宠</h2>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-primary" checked={draft.appearance.pet} onChange={e=>setDraft(t=>({...t,appearance:{...t.appearance,pet:e.target.checked}}))}/>在前台显示桌宠</label>
        <div className="space-y-2"><Label htmlFor="pet-model">VRM 模型地址</Label><Input id="pet-model" value={draft.pet.model_url} onChange={e=>setDraft(t=>({...t,pet:{...t.pet,model_url:e.target.value}}))} placeholder="站内路径或 HTTPS 链接"/><p className="text-sm text-muted-foreground">请填写有权使用的兼容模型地址。没有模型上传或动作编辑功能；地址为空时不显示桌宠。</p></div>
        <div className="space-y-2"><Label htmlFor="pet-label">桌宠名称</Label><Input id="pet-label" maxLength={80} value={draft.pet.label} onChange={e=>setDraft(t=>({...t,pet:{...t.pet,label:e.target.value}}))}/></div>
        <div className="space-y-2"><Label htmlFor="pet-default-state">首次进入网站时</Label><Select value={behavior?.pet_default_expanded?'expanded':'collapsed'} onValueChange={value=>setBehavior(current=>current?{...current,pet_default_expanded:value==='expanded'}:current)} disabled={!behavior||busy}><SelectTrigger id="pet-default-state"><SelectValue/></SelectTrigger><SelectContent><SelectItem value="expanded">展开桌宠</SelectItem><SelectItem value="collapsed">收起桌宠</SelectItem></SelectContent></Select><p className="text-sm text-muted-foreground">访客之后可以自行展开或收起。收起时不加载模型，手机端不显示桌宠。</p></div>
      </section>
    </TabsContent>
    <TabsContent value="emojis" className="space-y-6">
      <section className="space-y-3 rounded-xl border border-border bg-card p-5" aria-labelledby="reaction-heading"><h2 id="reaction-heading" className="text-lg font-semibold">贴表情互动</h2><label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-primary" disabled={!behavior||busy} checked={!!behavior?.allow_repeated_reactions} onChange={e=>setBehavior(current=>current?{...current,allow_repeated_reactions:e.target.checked}:current)}/>允许重复贴表情</label><p className="text-sm text-muted-foreground">开启后，同一张提问卡片可多次贴表情；关闭后，每个网络地址对每张卡片只能回应一次。点击底部“保存设置”后生效。</p></section>
      <EmojiSettings />
    </TabsContent>
    <TabsContent value="footer"><FooterSettings /></TabsContent>
    </Tabs>
    {['site','effects','emojis'].includes(area)&&<div className="sticky bottom-0 z-10 flex flex-wrap items-center justify-between gap-3 border-t border-border bg-background py-4"><p className="text-sm text-destructive" role={validation?'alert':undefined}>{validation}</p><div className="flex gap-2"><Button variant="outline" disabled={!dirty||busy} onClick={()=>{setDraft(copyTheme(saved));setBehavior(savedBehavior?{...savedBehavior}:null);setError('');setMessage('');}}>撤销修改</Button><Button disabled={!ready||busy||!!validation||!dirty||(themeDirty&&revision!==baseRevision)} onClick={()=>void save()}><Save size={16}/>{busy?'保存中…':'保存设置'}</Button></div></div>}
  </div>;
}
