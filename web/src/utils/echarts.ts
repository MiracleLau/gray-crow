import * as echarts from 'echarts/core'
import { BarChart, LineChart, PieChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TitleComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { EChartsCoreOption } from 'echarts/core'

echarts.use([LineChart, PieChart, BarChart, GridComponent, TooltipComponent, LegendComponent, TitleComponent, CanvasRenderer])

export { echarts }

export function baseOption(): EChartsCoreOption {
  return {
    backgroundColor: 'transparent',
    textStyle: { color: '#8b9bb4', fontFamily: 'Inter, system-ui, sans-serif' },
    grid: { left: 44, right: 20, top: 34, bottom: 30 },
    tooltip: {
      backgroundColor: 'rgba(17,23,38,.94)',
      borderColor: 'rgba(0,229,255,.3)',
      textStyle: { color: '#e6f1ff', fontSize: 12 },
    },
    legend: { textStyle: { color: '#8b9bb4' }, top: 0, right: 0 },
  }
}

export function axisStyle() {
  return {
    axisLine: { lineStyle: { color: 'rgba(0,229,255,.2)' } },
    axisLabel: { color: '#8b9bb4' },
    splitLine: { lineStyle: { color: 'rgba(0,229,255,.08)' } },
  }
}

export function areaSeries(name: string, data: number[], color: string) {
  return {
    name,
    type: 'line' as const,
    smooth: true,
    symbol: 'none',
    data,
    lineStyle: { color, width: 2 },
    areaStyle: {
      color: {
        type: 'linear' as const,
        x: 0,
        y: 0,
        x2: 0,
        y2: 1,
        colorStops: [
          { offset: 0, color: color.replace(')', ', .35)').replace('rgb', 'rgba') },
          { offset: 1, color: 'rgba(0,229,255,0)' },
        ],
      },
    },
  }
}
