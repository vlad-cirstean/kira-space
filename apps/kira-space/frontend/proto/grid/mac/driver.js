// Scroll driver for wkhost: idle, then three sustained velocity bands. Runs in the page, 4 s after
// load. Works on all four pages: Cheetah and the empty control use `.grid-scrollable`, SlickGrid
// its viewport. Marks go out through document.title (`MARK-*`).
(async () => {
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const frame = () => new Promise((resolve) => requestAnimationFrame(resolve));
  const mark = (name) => {
    document.title = `MARK-${name}`;
  };
  const scroller =
    document.querySelector('.grid-scrollable') ??
    document.querySelector('.slick-viewport-top.slick-viewport-right');
  if (!scroller) {
    mark('noscroller');
    return;
  }
  await sleep(4000);
  mark('idle');
  await sleep(4000);
  for (const px of [40, 100, 200]) {
    scroller.scrollTop = 0;
    await sleep(2000);
    mark(`${px}px`);
    // Sustained: the same px every frame for 60 frames, so velocity (not distance) varies.
    for (let i = 0; i < 60; i++) {
      scroller.scrollTop += px;
      await frame();
    }
    await sleep(1000);
  }
  mark('done');
})();
