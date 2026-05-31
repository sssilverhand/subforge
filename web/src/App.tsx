import { Routes, Route, Navigate } from 'react-router-dom'
import LoginPage from './pages/Login'
import Layout from './components/Layout'
import DashboardPage from './pages/Dashboard'
import SubscriptionsPage from './pages/Subscriptions'
import NodesPage from './pages/Nodes'
import UsersPage from './pages/Users'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const token = localStorage.getItem('token')
  if (!token) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Navigate to="/dashboard" replace />} />
        <Route path="dashboard" element={<DashboardPage />} />
        <Route path="subscriptions" element={<SubscriptionsPage />} />
        <Route path="nodes" element={<NodesPage />} />
        <Route path="users" element={<UsersPage />} />
      </Route>
    </Routes>
  )
}
