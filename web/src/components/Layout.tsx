import { Outlet, NavLink, useNavigate } from 'react-router-dom'
import { LayoutDashboard, ListChecks, Server, Users, LogOut } from 'lucide-react'

const nav = [
  { to: '/dashboard', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/subscriptions', icon: ListChecks, label: 'Subscriptions' },
  { to: '/nodes', icon: Server, label: 'Nodes' },
  { to: '/users', icon: Users, label: 'Users' },
]

export default function Layout() {
  const navigate = useNavigate()

  const logout = () => {
    localStorage.removeItem('token')
    navigate('/login')
  }

  return (
    <div className="flex h-screen">
      {/* Sidebar */}
      <aside className="w-56 bg-card border-r border-border flex flex-col">
        <div className="px-6 py-5 border-b border-border">
          <span className="text-lg font-bold tracking-tight">⚡ SubForge</span>
        </div>
        <nav className="flex-1 p-3 space-y-1">
          {nav.map(({ to, icon: Icon, label }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                  isActive
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted'
                }`
              }
            >
              <Icon size={16} />
              {label}
            </NavLink>
          ))}
        </nav>
        <button
          onClick={logout}
          className="flex items-center gap-3 px-6 py-4 text-sm text-muted-foreground hover:text-foreground border-t border-border"
        >
          <LogOut size={16} /> Выйти
        </button>
      </aside>

      {/* Main */}
      <main className="flex-1 overflow-auto p-8">
        <Outlet />
      </main>
    </div>
  )
}
