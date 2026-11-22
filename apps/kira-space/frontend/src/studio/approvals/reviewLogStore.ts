import { defineStore } from 'pinia';
import { ref } from 'vue';

// How a held log reads: one set of options shared by every review dialog, kept for the session.
// Wrap is on because a review must not hide bytes behind a sideways scroll.
export const useReviewLogStore = defineStore('reviewLog', () => {
  const wrap = ref(true);
  const timestamps = ref(true);
  const lineNumbers = ref(false);
  return { wrap, timestamps, lineNumbers };
});
