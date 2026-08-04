// 格式化与阈值工具。纯函数，无状态。
// 单位约定：CPU 为 MHz，内存/磁盘为 MB。

/** formatMHz 将 MHz 值转为可读字符串（≥1000 时用 GHz）。 */
export function formatMHz(v: number): string {
  if (v >= 1000) return `${(v / 1000).toFixed(1)} GHz`
  return `${Math.round(v)} MHz`
}

/** formatMB 将 MB 值转为可读字符串（≥1024 时用 GB）。 */
export function formatMB(v: number): string {
  if (v >= 1024) return `${(v / 1024).toFixed(1)} GB`
  return `${Math.round(v)} MB`
}

/** formatPct 将 0–1 的比例转为百分比字符串；-1 表示无数据。 */
export function formatPct(u: number): string {
  if (u < 0) return 'n/a'
  return `${Math.round(u * 100)}%`
}

/**
 * utilization 计算利用率（0–1）。
 * capacity ≤ 0 或 used < 0 时返回 null（无数据）。
 */
export function utilization(used: number, capacity: number): number | null {
  if (capacity <= 0 || used < 0) return null
  return used / capacity
}

/**
 * level 根据利用率返回颜色等级。
 * null 或负数 → 'none'；≥90% → 'crit'；≥70% → 'warn'；其余 → 'ok'。
 */
export function level(u: number | null): 'ok' | 'warn' | 'crit' | 'none' {
  if (u === null || u < 0) return 'none'
  if (u >= 0.9) return 'crit'
  if (u >= 0.7) return 'warn'
  return 'ok'
}

/**
 * sparklinePoints 生成 SVG polyline 的 points 属性字符串。
 * 将 values 等比映射到 width×height 的坐标空间，Y 轴翻转（SVG 原点在左上）。
 */
export function sparklinePoints(values: number[], width: number, height: number): string {
  if (values.length === 0) return ''
  const max = Math.max(...values, 1) // 避免除零
  const stepX = values.length > 1 ? width / (values.length - 1) : 0
  return values
    .map((v, i) => {
      const x = (i * stepX).toFixed(1)
      const y = (height - (v / max) * height).toFixed(1)
      return `${x},${y}`
    })
    .join(' ')
}
