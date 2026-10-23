// Account creation is irreversible from the form's perspective. Failure after
// POST must send the user to login, not invite another registration attempt.
export async function completeRegistration(createAccount: () => Promise<unknown>, initializeSession: () => Promise<unknown>) {
    await createAccount();
    try { await initializeSession(); return { sessionReady: true }; }
    catch { return { sessionReady: false }; }
}
