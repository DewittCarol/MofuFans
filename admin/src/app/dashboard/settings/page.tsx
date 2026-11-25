"use client";
import { AISettingsPanel } from "@/components/ai-settings";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import Link from "next/link";
import { BilibiliVerificationSettings } from "@/components/bilibili-verification-settings";
import { SettingsNavigation } from "@/components/settings-navigation";
import { InstallationSettingsPanel } from "@/components/installation-settings";
import { getSettings, updateSettings } from "@/lib/api";

export default function SettingsPage() {
  const [requireVerifiedUserToPost, setRequireVerifiedUserToPost] = useState(false);
  const [accountsEnabled, setAccountsEnabled] = useState(true);
  const [voyagePublicEnabled, setVoyagePublicEnabled] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [isSuccess, setIsSuccess] = useState(false);

  useEffect(() => {
    const loadSettings = async () => {
      try {
        const res = await getSettings();
        if (res.code === 200) {
          setRequireVerifiedUserToPost(!!res.data.require_verified_user_to_post);
          setAccountsEnabled(!res.data.disable_member_accounts);
          setVoyagePublicEnabled(!!res.data.voyage_public_enabled);
        } else {
          setMessage(res.message || "加载失败");
        }
      } catch {
        setMessage("加载失败");
      } finally {
        setIsLoading(false);
      }
    };

    loadSettings();
  }, []);

  const handleSave = async () => {
    setIsSaving(true);
    setMessage("");
    setIsSuccess(false);
    try {
      const res = await updateSettings({
        require_verified_user_to_post: requireVerifiedUserToPost,
        disable_member_accounts: !accountsEnabled,
        voyage_public_enabled: voyagePublicEnabled,
      });
      if (res.code === 200) {
        setRequireVerifiedUserToPost(!!res.data.require_verified_user_to_post);
        setAccountsEnabled(!res.data.disable_member_accounts);
        setVoyagePublicEnabled(!!res.data.voyage_public_enabled);
        setMessage("保存成功");
        setIsSuccess(true);
      } else {
        setMessage(res.message || "保存失败");
      }
    } catch {
      setMessage("保存失败");
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <SettingsNavigation>
    <div className="space-y-8">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">
          系统设置
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">品牌、配色、背景、鼠标、桌宠、表情与页脚备案均可在 <Link className="text-primary underline underline-offset-4" href="/dashboard/themes/">主题管理</Link> 中配置。</p>
      </header>

      <InstallationSettingsPanel />

      <Card id="submission-settings" tabIndex={-1} aria-label="投稿权限">
        <CardHeader>
          <CardTitle>投稿权限</CardTitle>
          <CardDescription>修改后点击下方“保存设置”。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="flex max-w-2xl items-start gap-3">
            <Checkbox id="member-accounts-enabled" checked={accountsEnabled}
              onCheckedChange={checked => { setAccountsEnabled(!!checked); if (!checked) setRequireVerifiedUserToPost(false); }} disabled={isLoading || isSaving} />
            <div className="space-y-1">
              <Label htmlFor="member-accounts-enabled">开启前台登录 / 注册</Label>
              <p className="text-sm text-muted-foreground">关闭后暂停用户登录、注册和用户中心，投稿使用匿名方式。已有地址和领取记录保留，后台登录不受影响。</p>
            </div>
          </div>

          <div className="flex max-w-2xl items-start gap-3">
            <Checkbox
              id="require-verified-user"
              checked={requireVerifiedUserToPost}
              onCheckedChange={(checked) => setRequireVerifiedUserToPost(!!checked)}
              disabled={isLoading || isSaving || !accountsEnabled}
            />
            <div className="space-y-1">
              <Label htmlFor="require-verified-user">投稿需要已验证用户</Label>
              <p className="text-sm text-muted-foreground">
                开启后仅已完成 B 站验证并登录的用户可以投稿。
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card id="voyage-settings" tabIndex={-1} aria-label="舰礼展示">
        <CardHeader><CardTitle>舰礼展示</CardTitle></CardHeader>
        <CardContent>
          <div className="flex max-w-2xl items-start gap-3">
            <Checkbox id="voyage-public-enabled" checked={voyagePublicEnabled}
              onCheckedChange={checked => setVoyagePublicEnabled(!!checked)} disabled={isLoading || isSaving} />
            <div className="space-y-1">
              <Label htmlFor="voyage-public-enabled">在前台展示舰礼</Label>
              <p className="text-sm text-muted-foreground">开启后访客可查看已发布的舰礼；关闭后公开目录仅供管理员预览。已登录并完成签名验证的用户，仍可在用户中心查看已发布的本人领取批次和物流。点击“保存设置”后生效。</p>
            </div>
          </div>
        </CardContent>
      </Card>

      <div id="save-settings" tabIndex={-1} aria-label="保存设置" className="flex flex-wrap items-center gap-3">
        <Button onClick={handleSave} disabled={isLoading || isSaving}>
          {isLoading ? "加载中…" : isSaving ? "保存中…" : "保存设置"}
        </Button>
        {message && (
          <p className={isSuccess ? "text-success" : "text-destructive"}>
            {message}
          </p>
        )}
      </div>
      <BilibiliVerificationSettings />
      <AISettingsPanel />
    </div>
    </SettingsNavigation>
  );
}
