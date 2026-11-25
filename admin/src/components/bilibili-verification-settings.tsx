"use client";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function BilibiliVerificationSettings() {
  return (
    <Card id="bilibili-verification" tabIndex={-1} aria-label="B 站注册验证">
      <CardHeader>
        <CardTitle>B 站注册验证</CardTitle>
        <CardDescription>用户通过个人签名验证账号归属，完成后设置本站登录密码。</CardDescription>
      </CardHeader>
      <CardContent className="max-w-2xl space-y-3 text-sm leading-7">
        <p>首次注册时，用户将本站生成的验证文字临时加入 B 站签名。验证成功后即可恢复签名，之后使用 UID 和本站密码登录。</p>
        <p>已登录并完成签名验证的用户可管理自己的地址簿，查看本人已发布的领取批次和物流。是否可以领取，仍以后台名单为准。</p>
        <p className="text-muted-foreground">无需配置验证账号或 Cookie，也不再使用取关、重新关注的方式。主播无需逐人发送认证码；原私信认证码入口仍可使用。</p>
        <a className="inline-flex min-h-11 items-center font-medium text-primary underline underline-offset-4" href="/account/" target="_blank" rel="noreferrer">打开用户中心</a>
      </CardContent>
    </Card>
  );
}
