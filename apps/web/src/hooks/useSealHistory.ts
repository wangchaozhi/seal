import { useCallback, useRef, useState } from "react";

interface HistoryState<T> {
  past: T[];
  present: T;
  future: T[];
}

interface UpdateOptions {
  group?: string;
}

export type HistoryUpdate<T> = T | ((current: T) => T);

export function useSealHistory<T>(initialValue: T, limit = 40) {
  const [history, setHistory] = useState<HistoryState<T>>({
    past: [],
    present: initialValue,
    future: [],
  });
  const activeGroup = useRef<string | null>(null);

  const update = useCallback((value: HistoryUpdate<T>, options: UpdateOptions = {}) => {
    // Keep ref mutations outside React's state updater: StrictMode may invoke the
    // updater twice in development and must not turn the first grouped edit into
    // a continuation that loses its original snapshot.
    const continuesGroup = Boolean(options.group && activeGroup.current === options.group);
    activeGroup.current = options.group ?? null;
    setHistory((current) => {
      const next = typeof value === "function"
        ? (value as (current: T) => T)(current.present)
        : value;

      if (Object.is(next, current.present)) return current;

      if (continuesGroup) {
        return { ...current, present: next, future: [] };
      }

      return {
        past: [...current.past, current.present].slice(-limit),
        present: next,
        future: [],
      };
    });
  }, [limit]);

  const endGroup = useCallback(() => {
    activeGroup.current = null;
  }, []);

  const undo = useCallback(() => {
    activeGroup.current = null;
    setHistory((current) => {
      const previous = current.past.at(-1);
      if (previous === undefined) return current;

      return {
        past: current.past.slice(0, -1),
        present: previous,
        future: [current.present, ...current.future].slice(0, limit),
      };
    });
  }, [limit]);

  const redo = useCallback(() => {
    activeGroup.current = null;
    setHistory((current) => {
      const next = current.future[0];
      if (next === undefined) return current;

      return {
        past: [...current.past, current.present].slice(-limit),
        present: next,
        future: current.future.slice(1),
      };
    });
  }, [limit]);

  return {
    value: history.present,
    update,
    endGroup,
    undo,
    redo,
    canUndo: history.past.length > 0,
    canRedo: history.future.length > 0,
  };
}
