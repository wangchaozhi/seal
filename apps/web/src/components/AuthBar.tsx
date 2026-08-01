import { useEffect, useState } from "react";
import { currentUser, login, logout, oauthProviders, oauthStartURL, register, type OAuthProviders, type User } from "../lib/api";
import { SocialIcon } from "./SocialIcon";

export function AuthBar() {
  const [user, setUser] = useState<User | null>(null);
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("demo@example.com");
  const [password, setPassword] = useState("");
  const [mfaCode, setMfaCode] = useState("");
  const [message, setMessage] = useState("");
  const [providers, setProviders] = useState<OAuthProviders>({ qq: false, wechat: false, github: false, google: false });
  const [mode, setMode] = useState<"login" | "register">("login");

  useEffect(() => {
    const sync = () => void currentUser().then(setUser);
    sync();
    void oauthProviders().then(setProviders).catch(() => undefined);
    const oauthResult = new URLSearchParams(window.location.search).get("oauth");
    if (oauthResult) {
      if (oauthResult !== "success") setMessage("快捷登录未完成，请重试或使用邮箱登录");
      window.history.replaceState({}, "", `${window.location.pathname}${window.location.hash}`);
    }
    window.addEventListener("auth-changed", sync);
    return () => window.removeEventListener("auth-changed", sync);
  }, []);

  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    document.body.style.overflow = "hidden";
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  const authenticate = async (mode: "login" | "register") => {
    try {
      const next = mode === "login" ? await login(email, password, mfaCode) : await register(email, password, mfaCode);
      setUser(next);
      setPassword("");
      setMfaCode("");
      setMessage("");
      setOpen(false);
      window.dispatchEvent(new Event("auth-changed"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "账号操作失败");
    }
  };

  if (user) return (
    <div className="auth-bar signed-in">
      <span><b>{user.displayName || user.email}</b><small>{user.authProvider === "wechat" ? "微信登录" : user.authProvider === "qq" ? "QQ 登录" : user.authProvider === "github" ? "GitHub 登录" : user.authProvider === "google" ? "Google 登录" : user.membershipLevel === "vip" ? "VIP" : "免费账户"}</small></span>
      <button type="button" onClick={() => void logout().then(() => { setUser(null); window.dispatchEvent(new Event("auth-changed")); })}>退出</button>
    </div>
  );

  return (
    <div className="auth-bar">
      <button className="auth-trigger" type="button" aria-expanded={open} onClick={() => setOpen((value) => !value)}>登录 / 注册</button>
      {open && <div className="auth-modal-backdrop" onMouseDown={(event) => { if (event.currentTarget === event.target) setOpen(false); }}>
        <section className="auth-card" role="dialog" aria-modal="true" aria-labelledby="auth-title">
          <button className="auth-close" type="button" aria-label="关闭账号入口" onClick={() => setOpen(false)}>×</button>
          <div className="auth-logo" aria-hidden="true">印</div>
          <div className="auth-card-heading"><h2 id="auth-title">{mode === "login" ? "用户登录" : "创建账户"}</h2><p>{mode === "login" ? "欢迎回来，请登录您的账户" : "注册后可保存云端配置与高清任务"}</p></div>
          <div className="auth-form">
            <label><span>邮箱地址</span><input type="email" value={email} placeholder="请输入您的邮箱" autoComplete="email" onChange={(event) => setEmail(event.target.value)} /></label>
            <label><span>密码</span><input type="password" value={password} placeholder={mode === "login" ? "请输入密码" : "至少 10 位密码"} minLength={10} maxLength={128} autoComplete={mode === "login" ? "current-password" : "new-password"} onChange={(event) => setPassword(event.target.value)} /></label>
            <label className="mfa-field"><span>管理员动态码 <small>普通用户留空</small></span><input value={mfaCode} placeholder="6 位动态码" inputMode="numeric" pattern="[0-9]{6}" maxLength={6} autoComplete="one-time-code" onChange={(event) => setMfaCode(event.target.value.replace(/\D/g, ""))} /></label>
            {message && <p className="auth-error">{message}</p>}
            <button className="auth-submit" type="button" onClick={() => void authenticate(mode)}>{mode === "login" ? "登录" : "立即注册"}</button>
          </div>
          <div className="auth-divider"><span>第三方账号登录</span></div>
          <div className="social-login social-login-icons" aria-label="快捷登录">
            <button className="qq" type="button" disabled={!providers.qq} aria-label="使用 QQ 登录" title={providers.qq ? "使用 QQ 登录" : "QQ 登录尚未配置"} onClick={() => window.location.assign(oauthStartURL("qq"))}><SocialIcon provider="qq" /></button>
            <button className="wechat" type="button" disabled={!providers.wechat} aria-label="使用微信扫码登录" title={providers.wechat ? "使用微信扫码登录" : "微信登录尚未配置"} onClick={() => window.location.assign(oauthStartURL("wechat"))}><SocialIcon provider="wechat" /></button>
            <button className="github" type="button" disabled={!providers.github} aria-label="使用 GitHub 登录" title={providers.github ? "使用 GitHub 登录" : "GitHub 登录尚未配置"} onClick={() => window.location.assign(oauthStartURL("github"))}><SocialIcon provider="github" /></button>
            <button className="google" type="button" disabled={!providers.google} aria-label="使用 Google 登录" title={providers.google ? "使用 Google 登录" : "Google 登录尚未配置"} onClick={() => window.location.assign(oauthStartURL("google"))}><SocialIcon provider="google" /></button>
          </div>
          <p className="auth-switch">{mode === "login" ? "还没有账户？" : "已经有账户？"}<button type="button" onClick={() => { setMode((value) => value === "login" ? "register" : "login"); setMessage(""); }}>{mode === "login" ? "立即注册" : "返回登录"}</button></p>
        </section>
      </div>}
    </div>
  );
}
