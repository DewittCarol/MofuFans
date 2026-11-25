'use client';
import { useSiteTheme } from '@/lib/site-theme';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useEffect, useState, useTransition, type MouseEvent } from 'react';
import { LayoutDashboard, MessageSquare, Inbox, Trash2, Tag, Users, UserCheck, Settings, Palette, Home, Sailboat, Gift, ClipboardList, MapPin, Truck, LogOut, Music2, Images, ImagePlus, ChevronDown, BookOpen, LoaderCircle, Shield, type LucideIcon } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useAuth } from '@/lib/auth';

type Item={href:string;label:string;icon:LucideIcon};
const groups:{id:string;label:string;icon:LucideIcon;items:Item[]}[]=[
  {id:'questions',label:'提问管理',icon:MessageSquare,items:[
    {href:'/dashboard/questions/overview',label:'概览',icon:LayoutDashboard},
    {href:'/dashboard/questions',label:'收件箱',icon:Inbox},
    {href:'/dashboard/questions/spam',label:'拦截箱',icon:Trash2},
    {href:'/dashboard/tags',label:'话题管理',icon:Tag},
  ]},
  {id:'voyage',label:'大航海回馈',icon:Sailboat,items:[
    {href:'/dashboard/voyage/guide',label:'使用教程',icon:BookOpen},
    {href:'/dashboard/voyage',label:'舰礼配置',icon:Gift},
    {href:'/dashboard/voyage/roster',label:'名单管理',icon:ClipboardList},
    {href:'/dashboard/voyage/delivery',label:'地址收集',icon:MapPin},
    {href:'/dashboard/voyage/address-book',label:'地址簿',icon:BookOpen},
    {href:'/dashboard/voyage/shipping',label:'发货管理',icon:Truck},
  ]},
  {id:'gifts',label:'电子礼物',icon:Gift,items:[{href:'/dashboard/gifts/guide',label:'使用教程',icon:BookOpen},{href:'/dashboard/gifts',label:'礼物与发放',icon:Gift}]},
  {id:'songs',label:'歌单管理',icon:Music2,items:[{href:'/dashboard/songs',label:'歌曲列表',icon:Music2}]},
  {id:'gallery',label:'画廊管理',icon:Images,items:[
    {href:'/dashboard/gallery',label:'主播约稿',icon:ImagePlus},
    {href:'/dashboard/gallery/fans',label:'粉丝约稿',icon:Images},
  ]},
  {id:'accounts',label:'账户管理',icon:Users,items:[
    {href:'/dashboard/users',label:'管理员',icon:Users},
    {href:'/dashboard/members',label:'注册用户',icon:UserCheck},
  ]},
];

export function Sidebar(){
 const {theme}=useSiteTheme();
  const pathname=usePathname().replace(/\/$/,'');const {logout,user}=useAuth();
  const router=useRouter();
  const [isPending,startTransition]=useTransition();
  const [pendingHref,setPendingHref]=useState('');
  function navigate(event:MouseEvent<HTMLAnchorElement>,href:string){
    if(event.defaultPrevented||event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey||event.currentTarget.target==='_blank'||href===pathname)return;
    event.preventDefault();
    setPendingHref(href);
    startTransition(()=>router.push(href));
  }
  const activeGroup=groups.find(group=>group.items.some(item=>pathname===item.href))?.id;
  const [expanded,setExpanded]=useState<Record<string,boolean>>({});
  useEffect(()=>{if(activeGroup)setExpanded(current=>({...current,[activeGroup]:true}));},[activeGroup,pathname]);
  const link=(item:Item,nested=false)=>{const pending=isPending&&pendingHref===item.href;return <Link key={item.href} href={item.href} onClick={event=>navigate(event,item.href)} aria-busy={pending||undefined} aria-current={pathname===item.href?'page':undefined} className={cn('flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-ring',pathname===item.href||pending?'bg-sidebar-accent text-sidebar-foreground':'text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-foreground')}><item.icon className={nested?'h-4 w-4 shrink-0':'h-5 w-5 shrink-0'} aria-hidden="true"/><span className="flex-1">{item.label}</span>{pending&&<><LoaderCircle className="h-4 w-4 motion-safe:animate-spin" aria-hidden="true"/><span role="status" className="sr-only">正在打开{item.label}</span></>}</Link>;};
  return <aside className="flex h-full w-56 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground sm:w-60">
    <div className="flex h-14 shrink-0 items-center justify-center border-b border-sidebar-border px-4">{theme.brand.logo&&<img src={theme.brand.logo} alt="" className="site-brand-image mr-2 h-7 w-7 shrink-0 object-contain"/>}<span className="truncate text-base font-semibold tracking-tight">{theme.brand.admin_title}</span></div>
    <nav aria-label="管理菜单" className="flex-1 space-y-1 overflow-y-auto p-3">
      {groups.map(group=>{const open=expanded[group.id]??activeGroup===group.id;return <section key={group.id}>
        <button type="button" aria-expanded={open} aria-controls={`nav-${group.id}`} onClick={()=>setExpanded(current=>({...current,[group.id]:!open}))} className={cn('flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm font-medium hover:bg-sidebar-accent focus-visible:outline-2 focus-visible:outline-ring',activeGroup===group.id?'text-sidebar-foreground':'text-sidebar-foreground/85')}><group.icon className="h-5 w-5 shrink-0" aria-hidden="true"/><span className="flex-1">{group.label}</span><ChevronDown size={15} aria-hidden="true" className={cn('transition-transform motion-reduce:transition-none',!open&&'-rotate-90')}/></button>
        <div id={`nav-${group.id}`} hidden={!open} className="mb-2 ml-5 border-l border-sidebar-border pl-2">{group.items.map(item=>link(item,true))}</div>
      </section>;})}
      {link({href:'/dashboard/themes',label:'主题管理',icon:Palette})}
      {link({href:'/dashboard/settings',label:'系统设置',icon:Settings})}
      {user?.is_super_admin&&link({href:'/dashboard/security',label:'安全',icon:Shield})}
    </nav>
    <div className="space-y-1 border-t border-sidebar-border p-3"><a href="/" target="_blank" rel="noopener noreferrer" className="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-sidebar-foreground/80 hover:bg-sidebar-accent focus-visible:outline-2 focus-visible:outline-ring"><Home size={20}/>返回前台</a><button type="button" onClick={logout} className="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm text-sidebar-foreground/80 hover:bg-sidebar-accent focus-visible:outline-2 focus-visible:outline-ring"><LogOut size={20}/>注销</button></div>
  </aside>;
}
