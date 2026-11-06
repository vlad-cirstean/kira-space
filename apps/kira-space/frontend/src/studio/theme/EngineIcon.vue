<script setup lang="ts">
import type { ConnectionKind } from '@shared/domain/connection';
import {
  siApachekafka,
  siClickhouse,
  siKibana,
  siMariadb,
  siMongodb,
  siMysql,
  siPostgresql,
  siRedis,
  siSqlite,
} from 'simple-icons';
import { computed } from 'vue';

// The products' own marks, imported from simple-icons (see NOTICES.md) and
// drawn in each icon's real brand `hex`, not currentColor. Each mark is authored on a 24x24 grid;
// `translate(2,2) scale(0.5)` maps that onto this component's 16x16 canvas. SQS and S3 have no
// simple-icons entry (Amazon publishes no per-service marks there either, only a generic AWS
// wordmark), so they stay as original redrawn shapes — drawn in a fixed color instead of
// currentColor (SQS: dark navy #232F3E, S3: goldenrod #B8860B), since the halo below breaks the
// assumption that let them adopt whatever text/accent color ambient context set.
// Every mark is drawn twice: first a halo copy (same geometry, filled+stroked #ececec, wider
// stroke-width) sitting behind the real colored glyph, giving a thin off-white outline that hugs
// the icon's actual silhouette instead of a generic rounded-square chip — several brand hexes
// (MariaDB, SQLite, Kafka) are otherwise near-invisible on Kira's dark chrome, and a flat chip
// (tried first, in rounded-square and various tint forms) covered too much of the small glyph and
// read as a blocky sticker rather than the mark itself. #ececec was kept as the halo color (over
// plain #fff, which read too bright/harsh).
// Unlike Devicon, the 8 simple-icons imports are genuine ES imports (simple-icons
// ships real named exports and "sideEffects": false), so only the 8 referenced consts should end
// up in the bundle.
// 1:1 with parts/_icons.html's i-* engine symbols.
const props = defineProps<{ kind: ConnectionKind; size?: number }>();

const iconSize = computed(() => props.size ?? 16);
</script>

<template>
  <svg class="icon" :width="iconSize" :height="iconSize" viewBox="0 0 16 16" aria-hidden="true">
    <template v-if="kind === 'postgres'">
      <path
        transform="translate(2,2) scale(0.5)"
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        stroke-linejoin="round"
        :d="siPostgresql.path"
      />
      <path
        transform="translate(2,2) scale(0.5)"
        :fill="`#${siPostgresql.hex}`"
        :d="siPostgresql.path"
      />
    </template>

    <template v-if="kind === 'mariadb'">
      <path
        transform="translate(2,2) scale(0.5)"
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        stroke-linejoin="round"
        :d="siMariadb.path"
      />
      <path transform="translate(2,2) scale(0.5)" :fill="`#${siMariadb.hex}`" :d="siMariadb.path" />
    </template>

    <template v-if="kind === 'mysql'">
      <path
        transform="translate(2,2) scale(0.5)"
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        stroke-linejoin="round"
        :d="siMysql.path"
      />
      <path transform="translate(2,2) scale(0.5)" :fill="`#${siMysql.hex}`" :d="siMysql.path" />
    </template>

    <template v-if="kind === 'sqlite'">
      <path
        transform="translate(2,2) scale(0.5)"
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        stroke-linejoin="round"
        :d="siSqlite.path"
      />
      <path transform="translate(2,2) scale(0.5)" :fill="`#${siSqlite.hex}`" :d="siSqlite.path" />
    </template>

    <template v-if="kind === 'mongodb'">
      <path
        transform="translate(2,2) scale(0.5)"
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        stroke-linejoin="round"
        :d="siMongodb.path"
      />
      <path transform="translate(2,2) scale(0.5)" :fill="`#${siMongodb.hex}`" :d="siMongodb.path" />
    </template>

    <template v-if="kind === 'redis'">
      <path
        transform="translate(2,2) scale(0.5)"
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        stroke-linejoin="round"
        :d="siRedis.path"
      />
      <path transform="translate(2,2) scale(0.5)" :fill="`#${siRedis.hex}`" :d="siRedis.path" />
    </template>

    <template v-if="kind === 'kafka'">
      <path
        transform="translate(2,2) scale(0.5)"
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        stroke-linejoin="round"
        :d="siApachekafka.path"
      />
      <path
        transform="translate(2,2) scale(0.5)"
        :fill="`#${siApachekafka.hex}`"
        :d="siApachekafka.path"
      />
    </template>

    <template v-if="kind === 'sqs'">
      <path
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        fill-rule="evenodd"
        d="M6.6 3.9h7.1c.72 0 1.3.58 1.3 1.3v5.6c0 .72-.58 1.3-1.3 1.3H6.6c-.72 0-1.3-.58-1.3-1.3V5.2c0-.72.58-1.3 1.3-1.3zm.5 1.55v5.1h6.1v-5.1l-3.05 2.45z"
      />
      <path
        fill="none"
        stroke="#ececec"
        stroke-width="2.7"
        stroke-linecap="round"
        d="M1.2 5.9h2.5M1.2 8.1h2.5M1.2 10.3h2.5"
      />
      <path
        fill="#232F3E"
        fill-rule="evenodd"
        d="M6.6 3.9h7.1c.72 0 1.3.58 1.3 1.3v5.6c0 .72-.58 1.3-1.3 1.3H6.6c-.72 0-1.3-.58-1.3-1.3V5.2c0-.72.58-1.3 1.3-1.3zm.5 1.55v5.1h6.1v-5.1l-3.05 2.45z"
      />
      <path
        fill="none"
        stroke="#232F3E"
        stroke-width="1.3"
        stroke-linecap="round"
        d="M1.2 5.9h2.5M1.2 8.1h2.5M1.2 10.3h2.5"
      />
    </template>

    <template v-if="kind === 'clickhouse'">
      <path
        transform="translate(2,2) scale(0.5)"
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        stroke-linejoin="round"
        :d="siClickhouse.path"
      />
      <path
        transform="translate(2,2) scale(0.5)"
        :fill="`#${siClickhouse.hex}`"
        :d="siClickhouse.path"
      />
    </template>

    <template v-if="kind === 'kibana'">
      <path
        transform="translate(2,2) scale(0.5)"
        fill="#ececec"
        stroke="#ececec"
        stroke-width="1.4"
        stroke-linejoin="round"
        :d="siKibana.path"
      />
      <path transform="translate(2,2) scale(0.5)" :fill="`#${siKibana.hex}`" :d="siKibana.path" />
    </template>

    <template v-if="kind === 's3'">
      <path
        fill="none"
        stroke="#ececec"
        stroke-width="2.6"
        stroke-linejoin="round"
        d="M2.95 3.5l.9 9.25c.06.63.6 1.1 1.24 1.1h5.82c.64 0 1.18-.47 1.24-1.1l.9-9.25"
      />
      <ellipse cx="8" cy="3.4" rx="5.05" ry="1.7" fill="none" stroke="#ececec" stroke-width="2.6" />
      <path
        fill="none"
        stroke="#B8860B"
        stroke-width="1.2"
        stroke-linejoin="round"
        d="M2.95 3.5l.9 9.25c.06.63.6 1.1 1.24 1.1h5.82c.64 0 1.18-.47 1.24-1.1l.9-9.25"
      />
      <ellipse cx="8" cy="3.4" rx="5.05" ry="1.7" fill="none" stroke="#B8860B" stroke-width="1.2" />
    </template>
  </svg>
</template>
