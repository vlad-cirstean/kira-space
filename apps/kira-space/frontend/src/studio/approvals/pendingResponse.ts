import { dbMcpPendingResponseSchema } from '@shared/domain/dbmcp';
import { useQuery } from '@tanstack/vue-query';
import { computed } from 'vue';
import { control } from '../bridge/control';

/** The held result of a response-stage approval. Every body of the kind shares this one query. */
export function usePendingResponse(requestId: () => string) {
  return useQuery({
    queryKey: computed(() => ['dbMcpPendingResponse', requestId()] as const),
    queryFn: async () =>
      dbMcpPendingResponseSchema.parse(await control.dbMcpPendingResponse(requestId())),
    gcTime: 0, // MB-sized text, immutable per request id: drop on close
  });
}
