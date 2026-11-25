"use client";

import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { MessageSquare, Tag, Users, Image, Rainbow, Archive, Globe } from "lucide-react";
import { getConfig, updateConfig, getStatistics, Statistics } from "@/lib/api";

export default function OverviewPage() {
  const [announcement, setAnnouncement] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState("");
  const [stats, setStats] = useState<Statistics | null>(null);
  const [statsLoading, setStatsLoading] = useState(true);

  useEffect(() => {
    void loadConfig();
    void loadStatistics();
  }, []);

  const loadConfig = async () => {
    try {
      const res = await getConfig();
      if (res.code === 200) setAnnouncement(res.data.announcement || "");
    } catch (error) {
      console.error("Failed to load config:", error);
    }
  };

  const loadStatistics = async () => {
    setStatsLoading(true);
    try {
      const res = await getStatistics();
      if (res.code === 200) setStats(res.data);
    } catch (error) {
      console.error("Failed to load statistics:", error);
    } finally {
      setStatsLoading(false);
    }
  };

  const handleSave = async () => {
    setIsLoading(true);
    setMessage("");
    try {
      const res = await updateConfig({ announcement });
      setMessage(res.code === 200 ? "保存成功" : res.message || "保存失败");
    } catch {
      setMessage("保存失败");
    } finally {
      setIsLoading(false);
    }
  };

  const statCards = [
    { title: "提问总数", value: stats?.total_questions ?? 0, icon: MessageSquare, color: "text-primary" },
    { title: "话题数量", value: stats?.total_tags ?? 0, icon: Tag, color: "text-chart-2" },
    { title: "管理员数", value: stats?.total_users ?? 0, icon: Users, color: "text-chart-3" },
    { title: "注册用户", value: stats?.total_members ?? 0, icon: Users, color: "text-chart-2" },
    { title: "附图总数", value: stats?.total_images ?? 0, icon: Image, color: "text-chart-4" },
    { title: "彩虹提问", value: stats?.rainbow_questions ?? 0, icon: Rainbow, color: "text-chart-1" },
    { title: "已归档", value: stats?.archived_questions ?? 0, icon: Archive, color: "text-muted-foreground" },
    { title: "已公开", value: stats?.published_questions ?? 0, icon: Globe, color: "text-chart-5" },
  ];

  return (
    <div className="space-y-8">
      <header><h1 className="text-2xl font-semibold tracking-tight text-foreground">概览</h1></header>
      <section className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
        {statCards.map((stat) => {
          const Icon = stat.icon;
          return (
            <Card key={stat.title} className="py-4">
              <CardContent className="flex items-center gap-4 px-4">
                <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted ${stat.color}`}><Icon className="h-5 w-5" aria-hidden="true" /></div>
                <div className="min-w-0"><p className="truncate text-sm text-muted-foreground">{stat.title}</p><p className="text-2xl font-semibold text-foreground">{statsLoading ? "-" : stat.value}</p></div>
              </CardContent>
            </Card>
          );
        })}
      </section>
      {stats && stats.tag_stats.length > 0 && (
        <Card>
          <CardHeader><CardTitle>话题分布</CardTitle></CardHeader>
          <CardContent><div className="space-y-3">
            {stats.tag_stats.sort((a, b) => b.count - a.count).map(({ tag, count }) => {
              const percentage = stats.total_questions > 0 ? (count / stats.total_questions) * 100 : 0;
              return <div key={tag.id} className="space-y-1.5"><div className="flex items-center justify-between gap-3 text-sm"><span className="font-medium text-foreground">{tag.tag_name}</span><span className="text-muted-foreground">{count} 条 ({percentage.toFixed(1)}%)</span></div><div className="h-2 w-full overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary transition-all duration-300" style={{ width: `${percentage}%` }} /></div></div>;
            })}
          </div></CardContent>
        </Card>
      )}
      <Card>
        <CardHeader><CardTitle>公告设置</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2"><Label htmlFor="announcement">公告内容</Label><Textarea id="announcement" value={announcement} onChange={(event) => setAnnouncement(event.target.value)} placeholder="输入公告内容..." rows={5} /></div>
          {message && <div role="status" className={message === "保存成功" ? "text-success" : "text-destructive"}>{message}</div>}
          <Button onClick={handleSave} disabled={isLoading}>{isLoading ? "保存中..." : "保存"}</Button>
        </CardContent>
      </Card>
    </div>
  );
}
