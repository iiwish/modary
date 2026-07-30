import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  Braces,
  ChevronRight,
  LogOut,
  Menu,
  ShieldCheck,
  UserRound,
  X,
} from "lucide-react";
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { APIError, getSession, login, logout, type Session } from "./api";
import { moduleNavigation } from "./generated/navigation";
import { moduleRoutes } from "./generated/routes";

export function App() {
  const session = useQuery({ queryKey: ["session"], queryFn: getSession });
  if (session.isPending) return <LoadingScreen />;
  if (session.isError) return <LoginPage />;
  return <AuthenticatedApp session={session.data} />;
}

function AuthenticatedApp({ session }: { session: Session }) {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      queryClient.clear();
      navigate("/");
    },
  });
  useEffect(() => {
    setSidebarOpen(false);
    window.scrollTo({ top: 0, left: 0 });
  }, [location.pathname]);
  const navigation = moduleNavigation.filter((route) => {
    const permission = route.navigation?.permission;
    return !permission || session.actor.permissions.includes(permission);
  });
  return (
    <div className="app-frame">
      <aside className={sidebarOpen ? "sidebar sidebar-open" : "sidebar"}>
        <div className="brand-lockup">
          <div className="brand-mark"><Braces size={18} /></div>
          <div><strong>Modary</strong><span>Rulary F0</span></div>
          <button className="icon-button sidebar-close" onClick={() => setSidebarOpen(false)} aria-label="Close navigation"><X size={18} /></button>
        </div>
        <nav className="primary-nav" onClick={() => setSidebarOpen(false)}>
          {navigation.map((route) => {
            const item = route.navigation!;
            const Icon = item.icon;
            return <NavItem key={route.id} to={route.path} icon={<Icon size={18} />} label={item.label} />;
          })}
        </nav>
        <div className="sidebar-footer">
          <div className="actor-summary">
            <UserRound size={18} />
            <div><strong>{session.actor.display_name}</strong><span>{session.actor.roles.join(", ")}</span></div>
          </div>
          <button className="icon-button" onClick={() => signOut.mutate()} aria-label="Sign out" title="Sign out"><LogOut size={18} /></button>
        </div>
      </aside>
      {sidebarOpen && <button className="sidebar-scrim" aria-label="Close navigation" onClick={() => setSidebarOpen(false)} />}
      <main className="main-surface">
        <header className="mobile-header">
          <button className="icon-button" onClick={() => setSidebarOpen(true)} aria-label="Open navigation"><Menu size={20} /></button>
          <strong>Modary</strong>
          <span className="status-dot" title="Runtime ready" />
        </header>
        <Routes>
          {moduleRoutes.map((route) => {
            const permission = route.navigation?.permission;
            const Component = route.Component;
            const element = !permission || session.actor.permissions.includes(permission)
              ? <Component actor={session.actor} />
              : <Navigate to="/rulary/rules" replace />;
            return <Route key={route.id} path={route.path} element={element} />;
          })}
          <Route path="*" element={<Navigate to="/rulary/rules" replace />} />
        </Routes>
      </main>
    </div>
  );
}

function NavItem({ to, icon, label }: { to: string; icon: ReactNode; label: string }) {
  return <NavLink to={to} className={({ isActive }) => isActive ? "nav-item active" : "nav-item"}>{icon}<span>{label}</span><ChevronRight size={15} /></NavLink>;
}

function LoginPage() {
  const queryClient = useQueryClient();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError("");
    const form = new FormData(event.currentTarget);
    try {
      const session = await login(String(form.get("username")), String(form.get("password")));
      queryClient.setQueryData(["session"], session);
    } catch (caught) {
      setError(caught instanceof APIError ? caught.message : "Sign in failed");
    } finally {
      setPending(false);
    }
  }
  return (
    <main className="login-page">
      <section className="login-panel">
        <div className="login-brand"><div className="brand-mark"><Braces size={20} /></div><span>Modary</span></div>
        <div className="login-heading"><ShieldCheck size={24} /><h1>Sign in</h1></div>
        <form onSubmit={submit}>
          <label>Username<input name="username" autoComplete="username" required autoFocus /></label>
          <label>Password<input name="password" type="password" autoComplete="current-password" required /></label>
          {error && <div className="form-error" role="alert">{error}</div>}
          <button className="button primary" disabled={pending}>{pending ? "Signing in…" : "Sign in"}</button>
        </form>
      </section>
    </main>
  );
}

function LoadingScreen() {
  return <main className="loading-screen"><Activity className="spin" size={24} /><span>Loading Modary</span></main>;
}
