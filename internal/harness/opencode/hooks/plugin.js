// Kira agent hook plugin for OpenCode. Reports session activity to Kira over a unix socket.
// Every handler swallows its errors and never waits on Kira: a hook must not slow or break the agent.
// Event names are assumptions pending a model-backed probe (P449, U12).
export const KiraHooks = async () => {
  const sock = process.env.KIRA_AGENT_HOOK_SOCKET;
  const token = process.env.KIRA_AGENT_HOOK_TOKEN;
  const terminal = process.env.KIRA_TERMINAL_ID;
  if (!sock || !token || !terminal) return {};
  const seenMessages = new Set();
  const post = (body) => {
    try {
      fetch('http://kira/hook', {
        unix: sock,
        method: 'POST',
        headers: {
          authorization: `Bearer ${token}`,
          'content-type': 'application/json',
          'x-kira-terminal': terminal,
        },
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(2000),
      }).catch(() => {});
    } catch {}
  };
  const sessionOf = (props) =>
    (props && (props.sessionID || (props.info && (props.info.sessionID || props.info.id)))) || '';
  return {
    event: async ({ event }) => {
      try {
        const props = event.properties || {};
        switch (event.type) {
          case 'session.created':
          case 'session.idle':
          case 'session.deleted':
            post({ type: event.type, sessionID: sessionOf(props) });
            break;
          case 'permission.updated':
            post({
              type: event.type,
              sessionID: sessionOf(props),
              message: String(props.title || ''),
            });
            break;
          case 'message.updated': {
            const info = props.info || {};
            if (info.role !== 'user' || !info.id || seenMessages.has(info.id)) break;
            seenMessages.add(info.id);
            post({ type: event.type, sessionID: sessionOf(props) });
            break;
          }
        }
      } catch {}
    },
    'tool.execute.before': async (input) => {
      try {
        post({
          type: 'tool.execute.before',
          sessionID: input.sessionID || '',
          tool: input.tool || '',
        });
      } catch {}
    },
    'tool.execute.after': async (input) => {
      try {
        post({
          type: 'tool.execute.after',
          sessionID: input.sessionID || '',
          tool: input.tool || '',
        });
      } catch {}
    },
  };
};
