import { BarChart, LineChart, PieChart, ScatterChart } from 'echarts/charts';
import {
  AriaComponent,
  BrushComponent,
  DataZoomInsideComponent,
  GridComponent,
  LegendComponent,
  ToolboxComponent,
  TooltipComponent,
} from 'echarts/components';
import { use } from 'echarts/core';
import { SVGRenderer } from 'echarts/renderers';

// Tree-shaken core. SVG so a captured chart is exact vector output and stays crisp at any zoom.
use([
  BarChart,
  LineChart,
  PieChart,
  ScatterChart,
  AriaComponent,
  DataZoomInsideComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  BrushComponent,
  ToolboxComponent,
  SVGRenderer,
]);

export { default as VChart } from 'vue-echarts';
