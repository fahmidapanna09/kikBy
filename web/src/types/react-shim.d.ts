// Minimal ambient typings for React 19 + react-dom/client.
//
// WHY THIS EXISTS: the build sandbox has React installed but not @types/react
// (npm registry is blocked). In a normal environment delete this file and run
// `npm i -D @types/react @types/react-dom` — the app code is written against the
// real React API and needs no other change. Props of OUR components are fully
// typed; only React's own event/DOM prop typings are intentionally loose here.

declare module "react" {
  export type ReactNode = any;
  export type ReactElement = any;
  export type Key = string | number;
  export type FC<P = {}> = (props: P) => ReactElement | null;
  export type PropsWithChildren<P = {}> = P & { children?: ReactNode };
  export type CSSProperties = { [k: string]: string | number | undefined };
  export type Dispatch<A> = (a: A) => void;
  export type SetStateAction<S> = S | ((prev: S) => S);
  export type RefObject<T> = { current: T | null };
  export type MutableRefObject<T> = { current: T };
  export type Ref<T> = ((el: T | null) => void) | RefObject<T> | null;
  export type EffectCallback = () => void | (() => void);
  export type DependencyList = readonly unknown[];

  export interface SyntheticEvent<T = any> {
    target: any;
    currentTarget: T;
    key: string;
    shiftKey: boolean;
    ctrlKey: boolean;
    metaKey: boolean;
    altKey: boolean;
    clientX: number;
    clientY: number;
    isComposing?: boolean;
    nativeEvent: any;
    preventDefault(): void;
    stopPropagation(): void;
  }
  export type ChangeEvent<T = any> = SyntheticEvent<T>;
  export type FormEvent<T = any> = SyntheticEvent<T>;
  export type MouseEvent<T = any> = SyntheticEvent<T>;
  export type KeyboardEvent<T = any> = SyntheticEvent<T>;
  export type FocusEvent<T = any> = SyntheticEvent<T>;
  export type UIEvent<T = any> = SyntheticEvent<T>;
  export type ClipboardEvent<T = any> = SyntheticEvent<T> & { clipboardData: DataTransfer };
  export type DragEvent<T = any> = SyntheticEvent<T> & { dataTransfer: DataTransfer };

  export function useState<S>(init: S | (() => S)): [S, Dispatch<SetStateAction<S>>];
  export function useState<S = undefined>(): [S | undefined, Dispatch<SetStateAction<S | undefined>>];
  export function useEffect(cb: EffectCallback, deps?: DependencyList): void;
  export function useLayoutEffect(cb: EffectCallback, deps?: DependencyList): void;
  export function useRef<T>(init: T): MutableRefObject<T>;
  export function useRef<T>(init: T | null): RefObject<T>;
  export function useRef<T = undefined>(): MutableRefObject<T | undefined>;
  export function useMemo<T>(factory: () => T, deps: DependencyList): T;
  export function useCallback<T extends (...a: any[]) => any>(fn: T, deps: DependencyList): T;
  export function useReducer<S, A>(reducer: (s: S, a: A) => S, init: S): [S, Dispatch<A>];
  export function useContext<T>(ctx: Context<T>): T;
  export function useId(): string;
  export function useDeferredValue<T>(v: T): T;
  export function useTransition(): [boolean, (cb: () => void) => void];
  export function useSyncExternalStore<T>(
    subscribe: (onChange: () => void) => () => void,
    getSnapshot: () => T,
    getServerSnapshot?: () => T,
  ): T;
  export function useImperativeHandle<T>(ref: Ref<T> | undefined, init: () => T, deps?: DependencyList): void;
  export function startTransition(cb: () => void): void;

  export interface Context<T> {
    Provider: FC<{ value: T; children?: ReactNode }>;
    Consumer: FC<{ children: (v: T) => ReactNode }>;
  }
  export function createContext<T>(def: T): Context<T>;
  export function memo<P>(c: FC<P>): FC<P>;
  export function forwardRef<T, P = {}>(
    render: (props: P, ref: Ref<T>) => ReactElement | null,
  ): FC<P & { ref?: Ref<T> }>;
  export function lazy<P>(loader: () => Promise<{ default: FC<P> }>): FC<P>;
  export function createElement(type: any, props?: any, ...children: any[]): ReactElement;
  export function isValidElement(x: unknown): boolean;
  export const Fragment: FC<{ children?: ReactNode }>;
  export const StrictMode: FC<{ children?: ReactNode }>;
  export const Suspense: FC<{ fallback?: ReactNode; children?: ReactNode }>;

  export class Component<P = {}, S = {}> {
    constructor(props: P);
    props: Readonly<P> & { children?: ReactNode };
    state: Readonly<S>;
    setState(s: Partial<S> | ((prev: S) => Partial<S>)): void;
    render(): ReactNode;
  }
}

declare module "react-dom/client" {
  export function createRoot(el: Element | DocumentFragment): {
    render(node: any): void;
    unmount(): void;
  };
}

declare module "react/jsx-runtime" {
  export const Fragment: any;
  export function jsx(type: any, props: any, key?: any): any;
  export function jsxs(type: any, props: any, key?: any): any;
  export namespace JSX {
    type Element = any;
    interface ElementClass { render(): any }
    interface ElementAttributesProperty { props: {} }
    interface ElementChildrenAttribute { children: {} }
    interface IntrinsicAttributes { key?: string | number }
    interface IntrinsicElements { [tag: string]: any }
  }
}

declare module "react-dom" {
  export function createPortal(children: unknown, container: Element | DocumentFragment): unknown;
}
