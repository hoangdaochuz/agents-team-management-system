import { createContext, useCallback, useContext, useState, type ReactNode } from "react";
import { Icon, type IconName } from "../../lib/icons";

interface ToastItem {
  id: number;
  msg: string;
  icon: IconName;
}

interface ToastCtx {
  toast: (msg: string, icon?: IconName) => void;
}

const Ctx = createContext<ToastCtx>({ toast: () => {} });

export function useToast() {
  return useContext(Ctx);
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);

  const toast = useCallback((msg: string, icon: IconName = "check") => {
    const id = Date.now() + Math.random();
    setItems((prev) => [...prev, { id, msg, icon }]);
    setTimeout(() => setItems((prev) => prev.filter((t) => t.id !== id)), 2900);
  }, []);

  return (
    <Ctx.Provider value={{ toast }}>
      {children}
      <div className="toast-wrap">
        {items.map((t) => (
          <div className="toast" key={t.id}>
            <span style={{ width: 16, height: 16, display: "inline-grid", placeItems: "center" }}>
              <Icon name={t.icon} size={16} />
            </span>
            <span>{t.msg}</span>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
}
