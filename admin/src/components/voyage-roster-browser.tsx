'use client';

import type {ReactNode} from 'react';
import {Tabs,TabsList,TabsTrigger,TabsContent} from '@/components/ui/tabs';

export type RosterCategory = 'monthly' | 'event';

export function RosterCategoryTabs({value,onChange,disabled,children}:{value:RosterCategory;onChange:(value:RosterCategory)=>void;disabled?:boolean;children:ReactNode}) {
  return <Tabs value={value} onValueChange={value=>onChange(value as RosterCategory)}>
    <TabsList aria-label="名单类别" className="mb-5">
      <TabsTrigger value="monthly" disabled={disabled}>大航海</TabsTrigger>
      <TabsTrigger value="event" disabled={disabled}>活动 / 抽奖</TabsTrigger>
    </TabsList>
    <TabsContent value={value}>{children}</TabsContent>
  </Tabs>;
}

export function groupRosterMonths<T extends {month:string}>(rows:T[]) {
  const groups=new Map<string,T[]>();
  for(const row of rows){const key=row.month||'';groups.set(key,[...(groups.get(key)||[]),row]);}
  return [...groups.entries()].sort(([a],[b])=>b.localeCompare(a)).map(([month,rows])=>({
    month,rows,title:month?`${month.slice(0,4)}年${Number(month.slice(5))}月`:'未分月份',
  }));
}
