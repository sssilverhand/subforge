import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listUsers, createUser, deleteUser, type AdminUser } from '@/lib/api'
import { Plus, Trash2, Shield, User } from 'lucide-react'

function CreateUserModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const [form, setForm] = useState({ username: '', password: '', role: 'operator' })
  const [err, setErr] = useState('')

  const create = useMutation({
    mutationFn: () => createUser(form),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['users'] }); onClose() },
    onError: (e: Error) => setErr(e.message),
  })

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-card border border-border rounded-lg w-full max-w-sm p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-semibold mb-4">Новый пользователь</h2>
        <div className="space-y-3">
          <div>
            <label className="text-sm mb-1 block">Логин</label>
            <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })}
              className="w-full px-3 py-2 rounded bg-muted border border-border text-sm focus:outline-none" />
          </div>
          <div>
            <label className="text-sm mb-1 block">Пароль</label>
            <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })}
              className="w-full px-3 py-2 rounded bg-muted border border-border text-sm focus:outline-none" />
          </div>
          <div>
            <label className="text-sm mb-1 block">Роль</label>
            <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}
              className="w-full px-3 py-2 rounded bg-muted border border-border text-sm focus:outline-none">
              <option value="admin">admin</option>
              <option value="operator">operator</option>
            </select>
          </div>
          {err && <p className="text-red-400 text-xs">{err}</p>}
        </div>
        <div className="flex gap-3 mt-4">
          <button onClick={onClose} className="flex-1 py-2 rounded border border-border text-sm hover:bg-muted">Отмена</button>
          <button onClick={() => create.mutate()} disabled={create.isPending}
            className="flex-1 py-2 rounded bg-primary text-primary-foreground text-sm font-medium hover:opacity-90 disabled:opacity-50">
            {create.isPending ? 'Создание...' : 'Создать'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default function UsersPage() {
  const [showCreate, setShowCreate] = useState(false)
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['users'], queryFn: listUsers })
  const users = data?.data.items ?? []

  const remove = useMutation({
    mutationFn: deleteUser,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })

  const roleIcon = (role: AdminUser['role']) =>
    role === 'super_admin' ? <Shield size={14} className="text-yellow-400" /> : <User size={14} className="text-blue-400" />

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold">Пользователи панели</h1>
        <button onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 px-4 py-2 rounded bg-primary text-primary-foreground text-sm font-medium hover:opacity-90">
          <Plus size={16} /> Добавить
        </button>
      </div>

      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-border text-left">
              <th className="px-4 py-3 text-xs text-muted-foreground font-medium">Пользователь</th>
              <th className="px-4 py-3 text-xs text-muted-foreground font-medium">Роль</th>
              <th className="px-4 py-3 text-xs text-muted-foreground font-medium">Создан</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr><td colSpan={4} className="px-4 py-8 text-center text-muted-foreground text-sm">Загрузка...</td></tr>
            )}
            {users.map((u) => (
              <tr key={u.id} className="border-b border-border hover:bg-muted/20">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    {roleIcon(u.role)}
                    <span className="text-sm font-medium">{u.username}</span>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <span className="text-xs px-2 py-0.5 rounded bg-muted">{u.role}</span>
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground">
                  {new Date(u.created_at).toLocaleDateString('ru-RU')}
                </td>
                <td className="px-4 py-3 text-right">
                  {u.role !== 'super_admin' && (
                    <button onClick={() => { if (confirm('Удалить?')) remove.mutate(u.id) }}
                      className="p-1.5 rounded hover:bg-muted text-red-400 hover:text-red-300">
                      <Trash2 size={14} />
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {showCreate && <CreateUserModal onClose={() => setShowCreate(false)} />}
    </div>
  )
}
