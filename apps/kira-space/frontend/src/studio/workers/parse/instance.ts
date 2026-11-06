import { createParseClient } from './client';
import ParseWorker from './parse.worker?worker';

/** The app-wide parse worker client; the worker starts on the first `run`. */
export const parseClient = createParseClient(() => new ParseWorker());
