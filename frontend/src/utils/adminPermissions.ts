export interface AdminPagePermission {
  key: string
  path: string
  labelKey: string
  aliases?: string[]
}

export const ADMIN_PAGE_PERMISSIONS: AdminPagePermission[] = [
  { key: 'dashboard', path: '/admin/dashboard', labelKey: 'nav.dashboard' },
  { key: 'ops', path: '/admin/ops', labelKey: 'nav.ops' },
  { key: 'users', path: '/admin/users', labelKey: 'nav.users' },
  { key: 'groups', path: '/admin/groups', labelKey: 'nav.groups' },
  { key: 'channels_pricing', path: '/admin/channels/pricing', labelKey: 'nav.channelPricing', aliases: ['/admin/channels'] },
  { key: 'channels_monitor', path: '/admin/channels/monitor', labelKey: 'nav.channelMonitor' },
  { key: 'subscriptions', path: '/admin/subscriptions', labelKey: 'nav.subscriptions' },
  { key: 'accounts', path: '/admin/accounts', labelKey: 'nav.accounts' },
  { key: 'announcements', path: '/admin/announcements', labelKey: 'nav.announcements' },
  { key: 'proxies', path: '/admin/proxies', labelKey: 'nav.proxies' },
  { key: 'risk_control', path: '/admin/risk-control', labelKey: 'nav.riskControl' },
  { key: 'redeem', path: '/admin/redeem', labelKey: 'nav.redeemCodes' },
  { key: 'promo_codes', path: '/admin/promo-codes', labelKey: 'nav.promoCodes' },
  { key: 'affiliates', path: '/admin/affiliates/invites', labelKey: 'nav.affiliateManagement', aliases: ['/admin/affiliates', '/admin/affiliates/rebates', '/admin/affiliates/transfers'] },
  { key: 'orders', path: '/admin/orders/dashboard', labelKey: 'nav.orderManagement', aliases: ['/admin/orders', '/admin/orders/plans'] },
  { key: 'usage', path: '/admin/usage', labelKey: 'nav.usage' },
  { key: 'settings', path: '/admin/settings', labelKey: 'nav.settings' }
]

export function adminPermissionForPath(path: string): AdminPagePermission | undefined {
  return ADMIN_PAGE_PERMISSIONS.find((item) => {
    if (path === item.path || path.startsWith(item.path + '/')) return true
    return item.aliases?.some((alias) => path === alias || path.startsWith(alias + '/'))
  })
}

export function firstAllowedAdminPath(pages?: string[] | null): string {
  const allowed = new Set(pages ?? [])
  return ADMIN_PAGE_PERMISSIONS.find((item) => allowed.has(item.key))?.path ?? '/dashboard'
}

export function canAccessAdminPath(role: string | undefined, pages: string[] | undefined, path: string): boolean {
  if (role === 'admin') return true
  if (role !== 'operator') return false
  const permission = adminPermissionForPath(path)
  return !!permission && (pages ?? []).includes(permission.key)
}
