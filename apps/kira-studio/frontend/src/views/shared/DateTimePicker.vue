<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';

// The app-owned month grid + clock steppers that replace the bare
// <input type="datetime-local"> — the last piece of OS-drawn UI left in the renderer.
// Pure content, no trigger and no PopoverPanel of its own:
// TimestampPane.vue owns the calendar button, the open/close state and the PopoverPanel wrapping
// this — the same trigger/content split ColumnsMenu.vue's own consumer already uses.
const props = defineProps<{ zone: 'local' | 'utc' }>();
const model = defineModel<Date>({ required: true });

interface Parts {
  y: number;
  mo: number;
  day: number;
  h: number;
  mi: number;
  s: number;
}

function partsOf(d: Date): Parts {
  return props.zone === 'utc'
    ? {
        y: d.getUTCFullYear(),
        mo: d.getUTCMonth(),
        day: d.getUTCDate(),
        h: d.getUTCHours(),
        mi: d.getUTCMinutes(),
        s: d.getUTCSeconds(),
      }
    : {
        y: d.getFullYear(),
        mo: d.getMonth(),
        day: d.getDate(),
        h: d.getHours(),
        mi: d.getMinutes(),
        s: d.getSeconds(),
      };
}

function buildDate(y: number, mo: number, day: number, h: number, mi: number, s: number): Date {
  return props.zone === 'utc'
    ? new Date(Date.UTC(y, mo, day, h, mi, s))
    : new Date(y, mo, day, h, mi, s);
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n));
}

const selected = computed(() => partsOf(model.value));
const today = computed(() => partsOf(new Date()));

// The label cycles three views of the same navigation, days -> months -> years,
// so reaching a date years away no longer costs dozens of single-month clicks. Picking a
// month or a year moves only *this* view state, never `selected` — the same rule prevMonth/
// nextMonth already follow below, just for a bigger jump.
type CalendarMode = 'days' | 'months' | 'years';
const mode = ref<CalendarMode>('days');

// The visible month is its own state, independent of the selected day — paging doesn't move the
// selection until a day cell is clicked. Reset to the value's own month (and back to the
// day grid) whenever the value moves to a different month from outside (e.g. typing a new date
// into the pane's text field, or the cell selection itself changing).
const viewYear = ref(selected.value.y);
const viewMonth = ref(selected.value.mo);
watch(
  () => [selected.value.y, selected.value.mo] as const,
  ([y, mo]) => {
    viewYear.value = y;
    viewMonth.value = mo;
    mode.value = 'days';
  },
);

function cycleMode(): void {
  mode.value = mode.value === 'days' ? 'months' : mode.value === 'months' ? 'years' : 'days';
}
function pickMonth(mo: number): void {
  viewMonth.value = mo;
  mode.value = 'days';
}
// A 16-year block, aligned to a multiple of 16 so paging never leaves the just-picked year
// stranded at a block edge.
const yearBlockStart = computed(() => Math.floor(viewYear.value / 16) * 16);
const yearBlock = computed(() => Array.from({ length: 16 }, (_, i) => yearBlockStart.value + i));
function pickYear(y: number): void {
  viewYear.value = y;
  mode.value = 'months';
}

const MONTH_KEYS = [
  'january',
  'february',
  'march',
  'april',
  'may',
  'june',
  'july',
  'august',
  'september',
  'october',
  'november',
  'december',
] as const;
const monthNames = computed(() =>
  MONTH_KEYS.map((m) => t(`studio.dateTimePicker.month.${m}.label`)),
);
const monthShortNames = computed(() =>
  MONTH_KEYS.map((m) =>
    m === 'may'
      ? t('studio.dateTimePicker.month.may.label')
      : t(`studio.dateTimePicker.monthShort.${m}.label`),
  ),
);
const weekdayLabels = computed(() =>
  (['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'] as const).map(
    (d) => t(`studio.dateTimePicker.weekday.${d}.label`),
  ),
);

function prevMonth(): void {
  if (viewMonth.value === 0) {
    viewMonth.value = 11;
    viewYear.value -= 1;
  } else {
    viewMonth.value -= 1;
  }
}
function nextMonth(): void {
  if (viewMonth.value === 11) {
    viewMonth.value = 0;
    viewYear.value += 1;
  } else {
    viewMonth.value += 1;
  }
}

// D33a: "the prev/next arrows stay and act on whatever the current mode pages" — one month in
// the day grid, one year in the month grid, one 16-year block in the year grid.
function pagePrev(): void {
  if (mode.value === 'days') prevMonth();
  else if (mode.value === 'months') viewYear.value -= 1;
  else viewYear.value -= 16;
}
function pageNext(): void {
  if (mode.value === 'days') nextMonth();
  else if (mode.value === 'months') viewYear.value += 1;
  else viewYear.value += 16;
}

interface DayCell {
  day: number;
  year: number;
  month: number;
  inMonth: boolean;
  isToday: boolean;
  isSelected: boolean;
}

// Pure calendar-day arithmetic, deliberately done in the UTC epoch-day domain regardless of
// `zone` — a calendar grid has no timezone of its own, only the *comparison* against `selected`/
// `today` below needs the zone-adjusted parts. Always 6 full weeks (42 cells) for a stable grid
// height across months rather than 5 vs. 6 depending on where the month falls.
const days = computed<DayCell[]>(() => {
  const y = viewYear.value;
  const mo = viewMonth.value;
  const firstWeekday = (new Date(Date.UTC(y, mo, 1)).getUTCDay() + 6) % 7; // Monday = 0
  const startMs = Date.UTC(y, mo, 1) - firstWeekday * 86_400_000;
  const cells: DayCell[] = [];
  for (let i = 0; i < 42; i++) {
    const cell = new Date(startMs + i * 86_400_000);
    const cy = cell.getUTCFullYear();
    const cmo = cell.getUTCMonth();
    const cday = cell.getUTCDate();
    cells.push({
      day: cday,
      year: cy,
      month: cmo,
      inMonth: cmo === mo && cy === y,
      isToday: cy === today.value.y && cmo === today.value.mo && cday === today.value.day,
      isSelected:
        cy === selected.value.y && cmo === selected.value.mo && cday === selected.value.day,
    });
  }
  return cells;
});

function pickDay(cell: DayCell): void {
  const p = selected.value;
  model.value = buildDate(cell.year, cell.month, cell.day, p.h, p.mi, p.s);
}

function pickNow(): void {
  model.value = new Date();
}

const labelText = computed(() => {
  if (mode.value === 'months') return String(viewYear.value);
  if (mode.value === 'years') return `${yearBlockStart.value}–${yearBlockStart.value + 15}`;
  return `${monthNames.value[viewMonth.value]} ${viewYear.value}`;
});
const pagePrevTitle = computed(() =>
  mode.value === 'days'
    ? t('studio.dateTimePicker.previousMonth.tooltip')
    : mode.value === 'months'
      ? t('studio.dateTimePicker.previousYear.tooltip')
      : t('studio.dateTimePicker.previousYears.tooltip'),
);
const pageNextTitle = computed(() =>
  mode.value === 'days'
    ? t('studio.dateTimePicker.nextMonth.tooltip')
    : mode.value === 'months'
      ? t('studio.dateTimePicker.nextYear.tooltip')
      : t('studio.dateTimePicker.nextYears.tooltip'),
);

// The three clock steppers, each a writable computed so <Input type="number"> can v-model
// straight onto it — the browser's own native number stepper handles the chrome, so nothing
// here needs to reinvent it, only clamp what a hand-typed value (as opposed to a stepper click,
// which stepUp/stepDown already keeps in range) can push out of range.
const hourText = computed<string>({
  get: () => String(selected.value.h).padStart(2, '0'),
  set: (v) => {
    const n = Number(v);
    if (!Number.isFinite(n)) return;
    const p = selected.value;
    model.value = buildDate(p.y, p.mo, p.day, clamp(n, 0, 23), p.mi, p.s);
  },
});
const minuteText = computed<string>({
  get: () => String(selected.value.mi).padStart(2, '0'),
  set: (v) => {
    const n = Number(v);
    if (!Number.isFinite(n)) return;
    const p = selected.value;
    model.value = buildDate(p.y, p.mo, p.day, p.h, clamp(n, 0, 59), p.s);
  },
});
const secondText = computed<string>({
  get: () => String(selected.value.s).padStart(2, '0'),
  set: (v) => {
    const n = Number(v);
    if (!Number.isFinite(n)) return;
    const p = selected.value;
    model.value = buildDate(p.y, p.mo, p.day, p.h, p.mi, clamp(n, 0, 59));
  },
});
</script>

<template>
  <div class="flex flex-col gap-1 p-1.5" data-testid="datetime-picker">
    <div class="flex items-center justify-between">
      <TooltipIconButton
        icon="chevron-left"
        :label="pagePrevTitle"
        :aria-label="t('common.action.previous.label')"
        data-testid="datetime-picker-prev-month"
        @click="pagePrev"
      />
      <Tooltip>
        <TooltipTrigger as-child>
          <!-- P42 D33a: a plain <button> now, so the label itself is the mode-cycling control --
               reset to look like the <span> it replaces rather than a bordered control. -->
          <Button
            type="button"
            variant="toolbar"
            size="kira"
            class="h-auto px-1 py-0.5 text-fg"
            data-testid="datetime-picker-month"
            @click="cycleMode"
          >
            {{ labelText }}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{ t('studio.dateTimePicker.cycleView.tooltip') }}</TooltipContent>
      </Tooltip>
      <TooltipIconButton
        icon="chevron-right"
        :label="pageNextTitle"
        :aria-label="t('common.action.next.label')"
        data-testid="datetime-picker-next-month"
        @click="pageNext"
      />
    </div>
    <div data-testid="datetime-picker-mode" :data-mode="mode">
      <template v-if="mode === 'days'">
        <div class="grid grid-cols-7 gap-0.5">
          <span
            v-for="w in weekdayLabels"
            :key="w"
            class="flex items-center justify-center text-subtle text-kira-sm h-4.5"
            >{{
              w
            }}</span
          >
        </div>
        <div class="grid grid-cols-7 gap-0.5">
          <Button
            v-for="cell in days"
            :key="`${cell.year}-${cell.month}-${cell.day}`"
            type="button"
            :variant="cell.isSelected ? 'toolbar-primary' : 'toolbar'"
            size="kira"
            class="h-5.5 w-full p-0 font-normal"
            data-testid="datetime-picker-day"
            :data-in-month="cell.inMonth"
            :data-selected="cell.isSelected"
            :class="[
              cell.isSelected ? '' : cell.inMonth ? 'text-fg' : 'text-subtle',
              cell.isToday ? 'border-border' : '',
            ]"
            @click="pickDay(cell)"
          >
            {{ cell.day }}
          </Button>
        </div>
      </template>
      <!-- The month grid (3x4) and year-block grid (4x4, item 19's "16-year block") share this
           layout — same day-cell chip, just a 4-column grid instead of a 7-column one (D33a). -->
      <div v-else-if="mode === 'months'" class="grid grid-cols-4 gap-0.5">
        <Button
          v-for="(_name, i) in monthNames"
          :key="i"
          type="button"
          :variant="i === viewMonth ? 'toolbar-primary' : 'toolbar'"
          size="kira"
          class="h-5.5 w-full p-0 font-normal"
          data-testid="datetime-picker-month-cell"
          :data-selected="i === viewMonth"
          @click="pickMonth(i)"
        >
          {{ monthShortNames[i] }}
        </Button>
      </div>
      <div v-else class="grid grid-cols-4 gap-0.5">
        <Button
          v-for="y in yearBlock"
          :key="y"
          type="button"
          :variant="y === viewYear ? 'toolbar-primary' : 'toolbar'"
          size="kira"
          class="h-5.5 w-full p-0 font-normal"
          data-testid="datetime-picker-year-cell"
          :data-selected="y === viewYear"
          @click="pickYear(y)"
        >
          {{ y }}
        </Button>
      </div>
    </div>
    <div class="flex items-center border-t border-border gap-0.5 pt-1">
      <Input
        v-model="hourText"
        type="number"
        min="0"
        max="23"
        size="kira"
        class="w-14"
        data-testid="datetime-picker-hour"
      />
      <span class="text-subtle">:</span>
      <Input
        v-model="minuteText"
        type="number"
        min="0"
        max="59"
        size="kira"
        class="w-14"
        data-testid="datetime-picker-minute"
      />
      <span class="text-subtle">:</span>
      <Input
        v-model="secondText"
        type="number"
        min="0"
        max="59"
        size="kira"
        class="w-14"
        data-testid="datetime-picker-second"
      />
      <TooltipIconButton
        icon="clock"
        :label="t('common.now.status')"
        data-testid="datetime-picker-now"
        @click="pickNow"
      />
    </div>
  </div>
</template>
