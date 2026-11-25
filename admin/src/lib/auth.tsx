'use client';

import { createContext, useContext, useState, useEffect, useRef, ReactNode, type FormEvent } from 'react';
import { User, getInfo, logout as apiLogout } from './api';
import md5 from 'md5';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

interface AuthContextType {
  user: User | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  login: (user: User) => void;
  logout: () => Promise<void>;
  checkAuth: () => Promise<boolean>;
  confirmPassword: (force?: boolean) => Promise<boolean>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const recentUntil = useRef(0);
  const pending = useRef<{promise:Promise<boolean>;resolve:(value:boolean)=>void}|null>(null);
  const [reauthOpen,setReauthOpen]=useState(false);
  const [password,setPassword]=useState('');
  const [reauthError,setReauthError]=useState('');
  const [reauthBusy,setReauthBusy]=useState(false);
  const finishReauth=(ok:boolean)=>{pending.current?.resolve(ok);pending.current=null;setReauthOpen(false);setPassword('');setReauthError('');};
  useEffect(()=>()=>{pending.current?.resolve(false);pending.current=null;},[]);
  const confirmPassword=(force=false):Promise<boolean>=>{
    if(!force&&Date.now()<recentUntil.current)return Promise.resolve(true);
    if(pending.current)return pending.current.promise;
    let resolve!:(value:boolean)=>void;const promise=new Promise<boolean>(r=>{resolve=r;});
    pending.current={promise,resolve};setPassword('');setReauthError('');setReauthOpen(true);return promise;
  };
  const reauthenticate=async(event:FormEvent)=>{
    event.preventDefault();if(reauthBusy)return;setReauthBusy(true);setReauthError('');
    try{
      const response=await fetch('/api/admin/reauth',{method:'POST',credentials:'include',cache:'no-store',headers:{'Content-Type':'application/json','X-MofuFans-Request':'settings'},body:JSON.stringify({password:md5(password)})});
      const result=await response.json();
      if(result.code!==200)throw new Error(result.message+(result.retry_after?`（约 ${result.retry_after} 秒后可重试）`:''));
      recentUntil.current=Date.now()+Math.min(300,result.data.valid_seconds)*1000;finishReauth(true);
    }catch(e){setReauthError(e instanceof Error?e.message:'验证失败，请重试');}finally{setReauthBusy(false);}
  };

  const checkAuth = async (): Promise<boolean> => {
    try {
      const res = await getInfo();
      if (res.code === 200) {
        setUser(res.data);
        return true;
      }
      setUser(null);
      return false;
    } catch {
      setUser(null);
      return false;
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    checkAuth();
  }, []);

  const login = (user: User) => {
    setUser(user);
    recentUntil.current=Date.now()+5*60*1000;
  };

  const logout = async () => {
    try {
      await apiLogout();
    } finally {
      setUser(null);
      recentUntil.current=0;finishReauth(false);
    }
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        isLoading,
        isAuthenticated: !!user,
        login,
        logout,
        checkAuth,
        confirmPassword,
      }}
    >
      {children}
      <Dialog open={reauthOpen} onOpenChange={open=>{if(!open&&!reauthBusy)finishReauth(false);}}>
        <DialogContent><DialogHeader><DialogTitle>确认管理员身份</DialogTitle><DialogDescription>修改账号或封禁设置前，请输入当前管理员密码。验证后五分钟内无需重复输入。</DialogDescription></DialogHeader>
          <form onSubmit={reauthenticate} className="space-y-4">
            <div className="space-y-2"><Label htmlFor="admin-reauth-password">当前管理员密码</Label><Input id="admin-reauth-password" type="password" autoComplete="current-password" value={password} onChange={e=>setPassword(e.target.value)} maxLength={256} required disabled={reauthBusy}/></div>
            {reauthError&&<p role="alert" className="text-sm text-destructive">{reauthError}</p>}
            <div className="flex justify-end gap-2"><Button type="button" variant="outline" disabled={reauthBusy} onClick={()=>finishReauth(false)}>取消</Button><Button type="submit" disabled={reauthBusy||!password}>{reauthBusy?'正在验证…':'验证并继续'}</Button></div>
          </form>
        </DialogContent>
      </Dialog>
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
