import { useContext } from 'react';
import { AppContext } from './AppContextInstance';
import type { AppContextType } from './AppContextTypes';

export function useApp(): AppContextType {
  const ctx = useContext(AppContext);
  if (!ctx) throw new Error('useApp must be used within an AppProvider');
  return ctx;
}
