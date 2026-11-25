"use client";
import { useSiteTheme } from "@/lib/site-theme";

import { useState, useEffect } from "react";
import md5 from "md5";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { login as apiLogin } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { installationAPI, installationError } from "@/lib/installation";

export default function LoginPage() {
 const {theme}=useSiteTheme();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const { login, isAuthenticated, isLoading: authLoading } = useAuth();
  const [setupChecked, setSetupChecked] = useState(false);

  useEffect(() => {
    installationAPI<{ required: boolean }>("setup/status").then(status => {
      if (status.required) window.location.replace("/admin/setup/");
      else setSetupChecked(true);
    }).catch(error => setError(installationError(error)));
  }, []);

  useEffect(() => {
    if (!authLoading && isAuthenticated) {
      window.location.replace("/admin/dashboard/");
    }
  }, [authLoading, isAuthenticated]);

  if (authLoading || !setupChecked) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="space-y-4 text-center text-muted-foreground" role="status">{error || "正在检查站点状态…"}{error && <Button className="ml-3" variant="outline" onClick={() => window.location.reload()}>重试</Button>}</div>
      </div>
    );
  }

  if (isAuthenticated) {
    return null;
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setIsLoading(true);

    try {
      const res = await apiLogin(username, md5(password));
      if (res.code === 200) {
        login(res.data);
        // 使用整页跳转，确保带上服务端设置的 cookie 并正确进入后台
        window.location.replace("/admin/dashboard/");
        return;
      } else {
        setError(res.message || "登录失败");
      }
    } catch {
      setError("登录失败，请检查网络连接");
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-background px-4 py-16 sm:py-20">
      <div className="w-full max-w-[380px]">
        <Card className="fabric-shadow fabric-border">
          <CardHeader className="pb-2 pt-6">
            <CardTitle className="text-xl text-foreground">
              {theme.brand.admin_title}
            </CardTitle>

          </CardHeader>
          <CardContent className="pb-6">
            <form onSubmit={handleSubmit} className="space-y-5">
              <div className="space-y-2">
                <Label htmlFor="username" className="text-foreground">
                  用户名
                </Label>
                <Input
                  id="username"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="请输入用户名"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password" className="text-foreground">
                  密码
                </Label>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="请输入密码"
                  required
                />
              </div>
              {error && <div className="text-destructive text-sm">{error}</div>}
              <Button type="submit" className="w-full" disabled={isLoading}>
                {isLoading ? "登录中..." : "登录"}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
