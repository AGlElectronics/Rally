import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { getBrand, type ApiBrand } from '@/api/brand';

export type Brand = Required<Pick<ApiBrand, 'productName'>> & ApiBrand;

type Ctx = {
  brand: Brand;
  loading: boolean;
};

// Fallback brand so the login screen renders even before the /api/v1/brand
// response arrives (or if the backend is briefly down). The product name
// comes from the build-time VITE_PRODUCT_NAME so a convention can rebrand
// without a backend hit; the API response takes precedence once it lands.
const fallback: Brand = {
  revision: 0,
  source: 'standalone',
  productName: (import.meta.env.VITE_PRODUCT_NAME as string | undefined) || 'Rally',
  organizationName: '',
  loginSubtitle: 'Events & convention management',
  loginButtonLabel: 'Continue with your organisation login',
  logoUrl: '',
  themeColor: '#18181b',
  colors: {
    primary: '#4f46e5',
    carbon: '#18181b',
    shadow: '#241e20',
  },
  cssVariables: {},
};

const BrandCtx = createContext<Ctx>({ brand: fallback, loading: true });

export function BrandProvider({ children }: { children: ReactNode }) {
  const [brand, setBrand] = useState<Brand>(fallback);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const ctrl = new AbortController();
    getBrand(ctrl.signal)
      .then((data) => {
        if (cancelled) return;
        setBrand({ ...fallback, ...data, productName: data.productName || fallback.productName });
      })
      .catch(() => {
        // Keep the fallback. Not having a brand package is non-fatal.
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
      ctrl.abort();
    };
  }, []);

  // Apply any CSS variable overrides from the brand package to <html> so
  // Tailwind variable-driven classes pick them up without a rebuild.
  useEffect(() => {
    if (!brand.cssVariables) return;
    const root = document.documentElement;
    for (const [name, value] of Object.entries(brand.cssVariables)) {
      root.style.setProperty(name, value);
    }
  }, [brand.cssVariables]);

  const value = useMemo(() => ({ brand, loading }), [brand, loading]);
  return <BrandCtx.Provider value={value}>{children}</BrandCtx.Provider>;
}

export function useBrand(): Ctx {
  return useContext(BrandCtx);
}
