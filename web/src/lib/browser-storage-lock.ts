/**
 * HTTP origins lack Web Locks. A dedicated IndexedDB database per lock name
 * keeps unrelated/nested locks independent while serializing tabs.
 */
export function withBrowserStorageLock<T>(name: string, operation: () => Promise<T>): Promise<T> {
    if (typeof navigator !== "undefined" && navigator.locks) return navigator.locks.request(name, operation);
    if (typeof indexedDB === "undefined") return Promise.reject(new Error("浏览器不支持安全存储，请使用支持 IndexedDB 的浏览器"));
    return new Promise<T>((resolve, reject) => {
        const open = indexedDB.open(`yingce:storage-mutex:${name}`, 1);
        open.onupgradeneeded = () => open.result.createObjectStore("mutex");
        open.onerror = () => reject(open.error);
        open.onsuccess = () => {
            const db = open.result;
            let finished = false;
            let started = false;
            let failed = false;
            let result: T;
            let failure: unknown;
            let tx: IDBTransaction;
            try {
                tx = db.transaction("mutex", "readwrite");
            } catch (error) {
                db.close();
                reject(error);
                return;
            }
            tx.oncomplete = () => {
                db.close();
                if (failed) reject(failure);
                else resolve(result);
            };
            tx.onabort = () => {
                db.close();
                reject(tx.error || new Error("存储锁事务中断，请重新加载资源"));
            };
            // Keep the transaction active while async storage/network work runs.
            // The first request's success means this tab owns the exclusive lock.
            const pump = () => {
                const request = tx.objectStore("mutex").get("lock");
                request.onsuccess = () => {
                    if (!started) {
                        started = true;
                        Promise.resolve().then(operation).then(
                            (value) => { result = value; finished = true; },
                            (error) => { failure = error; failed = true; finished = true; },
                        );
                    }
                    if (!finished) pump();
                };
            };
            pump();
        };
    });
}
