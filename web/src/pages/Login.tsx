import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { login } from '@/lib/api'

export default function LoginPage() {
  const navigate = useNavigate()
  const [form, setForm] = useState({ username: '', password: '' })
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      const { data } = await login(form.username, form.password)
      localStorage.setItem('token', data.token)
      navigate('/dashboard')
    } catch {
      setError('Неверный логин или пароль')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center">
      <div className="w-full max-w-sm">
        <div className="bg-card border border-border rounded-lg p-8 shadow-lg">
          <h1 className="text-2xl font-bold mb-2">⚡ SubForge</h1>
          <p className="text-muted-foreground text-sm mb-6">Войдите в панель управления</p>

          <form onSubmit={submit} className="space-y-4">
            <div>
              <label className="block text-sm mb-1">Логин</label>
              <input
                type="text"
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                className="w-full px-3 py-2 rounded-md bg-muted border border-border text-sm focus:outline-none focus:ring-1 focus:ring-primary"
                autoComplete="username"
                required
              />
            </div>
            <div>
              <label className="block text-sm mb-1">Пароль</label>
              <input
                type="password"
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                className="w-full px-3 py-2 rounded-md bg-muted border border-border text-sm focus:outline-none focus:ring-1 focus:ring-primary"
                autoComplete="current-password"
                required
              />
            </div>
            {error && <p className="text-red-400 text-sm">{error}</p>}
            <button
              type="submit"
              disabled={loading}
              className="w-full py-2 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:opacity-90 disabled:opacity-50 transition-opacity"
            >
              {loading ? 'Вход...' : 'Войти'}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}
