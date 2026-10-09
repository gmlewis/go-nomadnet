// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.util.Log

/**
 * Where Android's package installer reports an installation this appliance started.
 *
 * The install of Termux is not done by this appliance: it opens a session, hands over the
 * bytes and gives the system an intent to answer on, and the system does the confirming and
 * the installing. That intent has to be answered even when the appliance's screen is gone —
 * the person may be looking at the installer while the activity is stopped — so this is a
 * receiver declared in the manifest rather than one the activity registers.
 *
 * One of the answers has to be acted on rather than logged. When a person has to confirm the
 * install, the system answers with `STATUS_PENDING_USER_ACTION` and the screen to show in an
 * extra; not starting it is an install that stops there and says nothing.
 */
class InstallResultReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)
        val message = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE)
        if (status == PackageInstaller.STATUS_PENDING_USER_ACTION) {
            // The person has to say yes. This is the moment the system hands over the screen
            // that asks, and there is nothing else to do with it.
            @Suppress("DEPRECATION")
            val confirmation = intent.getParcelableExtra<Intent>(Intent.EXTRA_INTENT)
            if (confirmation == null) {
                Log.i(TAG, "the installer asked for a confirmation and gave no screen to show")
                return
            }
            confirmation.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            runCatching { context.startActivity(confirmation) }
                .onFailure { Log.i(TAG, "could not show the installer's confirmation: ${it.message}") }
            return
        }
        Log.i(TAG, "the install of ${TermuxPackage.PACKAGE} finished with status $status: $message")
    }

    companion object {
        private const val TAG = "gonomadnet"

        /**
         * The action the installer answers on.
         *
         * It carries this package's name because a broadcast action is a name anyone may use;
         * the receiver is declared unexported, so a name that looks like this one from another
         * application reaches nothing.
         */
        const val ACTION_INSTALL_RESULT = "com.gmlewis.gonomadnet.INSTALL_RESULT"

        /**
         * The intent the installer will answer this session on.
         *
         * The component is named explicitly and the receiver is unexported, which is what makes
         * the answer trustworthy: the system may always deliver to an explicit component, and
         * another application may never. `FLAG_MUTABLE` is required from Android 12 because the
         * system adds the status to the intent it sends back; without it the flags throw.
         */
        fun confirmation(context: Context, sessionId: Int): PendingIntent {
            val intent = Intent(ACTION_INSTALL_RESULT).setPackage(context.packageName)
            return PendingIntent.getBroadcast(
                context,
                sessionId,
                intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
            )
        }
    }
}
