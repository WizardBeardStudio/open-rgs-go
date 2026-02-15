using System;

namespace WizardBeardStudio.Rgs.Metadata
{
    public static class IdempotencyKeyFactory
    {
        public static string Create() => Guid.NewGuid().ToString("N");
    }
}
