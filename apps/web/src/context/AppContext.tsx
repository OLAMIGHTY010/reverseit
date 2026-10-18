import React, { useState, useEffect } from 'react';
import type { Role, ThemeMode, DensityMode } from '../types/domain';
import { AppContext } from './AppContextInstance';
import type { EnvironmentMode, ToastItem, PortalMode, StaffUser } from './AppContextTypes';
import { apiClient } from '../services/apiClient';

const AUTH_USER_KEY = 'smartreversal_auth_user';
const AUTH_TOKEN_KEY = 'smartreversal_auth_token';
const PORTAL_MODE_KEY = 'smartreversal_portal_mode';

export const AppProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [theme, setThemeState] = useState<ThemeMode>('daylight');
  const [density, setDensityState] = useState<DensityMode>('comfortable');
  const [role, setRole] = useState<Role>('ANALYST_MAKER');
  const [environment, setEnvironment] = useState<EnvironmentMode>('SIMULATED_SANDBOX');
  const [activeView, setActiveViewState] = useState<string>('dashboard');
  const [viewHistory, setViewHistory] = useState<string[]>([]);
  const [customerStep, setCustomerStepState] = useState<string>('S1_REPORT_PROBLEM');
  const [customerHistory, setCustomerHistory] = useState<string[]>([]);
  const [globalSearchOpen, setGlobalSearchOpen] = useState(false);
  const [sessionTimeoutOpen, setSessionTimeoutOpen] = useState(false);
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [selectedTransactionId, setSelectedTransactionId] = useState<string | null>(null);

  const setActiveView = (view: string) => {
    if (view !== activeView) {
      setViewHistory((prev) => [...prev, activeView]);
      setActiveViewState(view);
    }
  };

  const navigateBack = () => {
    if (viewHistory.length > 0) {
      const prevView = viewHistory[viewHistory.length - 1];
      setViewHistory((prev) => prev.slice(0, -1));
      setActiveViewState(prevView);
    } else if (activeView !== 'dashboard') {
      setActiveViewState('dashboard');
    }
  };

  const canGoBack = viewHistory.length > 0 || activeView !== 'dashboard';

  const setCustomerStep = (step: string) => {
    if (step !== customerStep) {
      setCustomerHistory((prev) => [...prev, customerStep]);
      setCustomerStepState(step);
    }
  };

  const navigateBackCustomer = () => {
    if (customerHistory.length > 0) {
      const prevStep = customerHistory[customerHistory.length - 1];
      setCustomerHistory((prev) => prev.slice(0, -1));
      setCustomerStepState(prevStep);
    } else if (customerStep !== 'S1_REPORT_PROBLEM') {
      setCustomerStepState('S1_REPORT_PROBLEM');
    }
  };

  const goCustomerHome = () => {
    setCustomerStepState('S1_REPORT_PROBLEM');
    setCustomerHistory([]);
  };

  const canGoBackCustomer = customerHistory.length > 0 || customerStep !== 'S1_REPORT_PROBLEM';

  // Portal mode: customer vs staff
  const [portalMode, setPortalModeState] = useState<PortalMode>(() => {
    const saved = localStorage.getItem(PORTAL_MODE_KEY);
    return saved === 'staff' ? 'staff' : 'customer';
  });

  // Auth state for staff
  const [currentUser, setCurrentUser] = useState<StaffUser | null>(() => {
    try {
      const saved = localStorage.getItem(AUTH_USER_KEY);
      return saved ? JSON.parse(saved) : null;
    } catch {
      return null;
    }
  });

  const [authToken, setAuthTokenState] = useState<string | null>(() => {
    return localStorage.getItem(AUTH_TOKEN_KEY);
  });

  const isAuthenticated = currentUser !== null;

  // Sync token to apiClient on mount or change
  useEffect(() => {
    apiClient.setAuthToken(authToken);
  }, [authToken]);

  const setPortalMode = (mode: PortalMode) => {
    setPortalModeState(mode);
    localStorage.setItem(PORTAL_MODE_KEY, mode);
  };

  const loginStaff = (user: StaffUser, token?: string) => {
    setCurrentUser(user);
    setRole(user.role);
    localStorage.setItem(AUTH_USER_KEY, JSON.stringify(user));
    if (token) {
      setAuthTokenState(token);
      localStorage.setItem(AUTH_TOKEN_KEY, token);
      apiClient.setAuthToken(token);
    }
    setPortalMode('staff');
  };

  const logoutStaff = () => {
    setCurrentUser(null);
    setAuthTokenState(null);
    localStorage.removeItem(AUTH_USER_KEY);
    localStorage.removeItem(AUTH_TOKEN_KEY);
    apiClient.setAuthToken(null);
  };

  // Sync theme with DOM root data attributes
  const setTheme = (newTheme: ThemeMode) => {
    setThemeState(newTheme);
    document.documentElement.setAttribute('data-theme', newTheme);
  };

  const setDensity = (newDensity: DensityMode) => {
    setDensityState(newDensity);
    document.documentElement.setAttribute('data-density', newDensity);
  };

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
    document.documentElement.setAttribute('data-density', density);
  }, [theme, density]);

  // Keyboard shortcut listener for Ctrl+K / Cmd+K
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setGlobalSearchOpen((prev) => !prev);
      }
      if (e.key === 'Escape') {
        setGlobalSearchOpen(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const dismissToast = (id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  };

  const addToast = (t: Omit<ToastItem, 'id'>) => {
    setToasts((prev) => {
      const id = `toast-${prev.length + 1}`;
      const newToast: ToastItem = { ...t, id };
      setTimeout(() => {
        dismissToast(id);
      }, 6000);
      return [...prev, newToast];
    });
  };

  return (
    <AppContext.Provider
      value={{
        theme,
        setTheme,
        density,
        setDensity,
        role,
        setRole,
        environment,
        setEnvironment,
        portalMode,
        setPortalMode,
        isAuthenticated,
        currentUser,
        authToken,
        loginStaff,
        logoutStaff,
        activeView,
        setActiveView,
        viewHistory,
        navigateBack,
        canGoBack,
        customerStep,
        setCustomerStep,
        customerHistory,
        navigateBackCustomer,
        goCustomerHome,
        canGoBackCustomer,
        globalSearchOpen,
        setGlobalSearchOpen,
        sessionTimeoutOpen,
        setSessionTimeoutOpen,
        toasts,
        addToast,
        dismissToast,
        selectedTransactionId,
        setSelectedTransactionId,
      }}
    >
      {children}
    </AppContext.Provider>
  );
};


/* eslint-disable-next-line react-refresh/only-export-components */
export { useApp } from './useApp';
